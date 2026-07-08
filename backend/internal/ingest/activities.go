package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"go.temporal.io/sdk/activity"
)

// Activities bundles the side-effecting steps of the pipeline with their
// dependencies. Registered on the worker; the workflow only ever calls these by
// reference, never touching the Source/Ollama/Qdrant/Postgres directly.
type Activities struct {
	Source Source
	Ollama *OllamaClient
	Qdrant *QdrantClient
	Store  *Store
}

// Plan is the reconcile diff between the Source (desired state) and the ledger
// (actual state): documents to (re)ingest and documents to purge.
type Plan struct {
	// Ingest holds new or changed documents.
	Ingest []Entry
	// Purge holds doc IDs that are in the ledger but no longer listed by the
	// Source — i.e. deletions.
	Purge []string
}

// Reconcile lists the Source, compares it against the ledger, and returns the
// work needed to converge them. This is the source of truth for deletes: a
// document in the ledger but absent from the listing is scheduled for purge.
// Results are sorted for stable, replay-friendly ordering downstream.
func (a *Activities) Reconcile(ctx context.Context) (Plan, error) {
	entries, err := a.Source.List(ctx)
	if err != nil {
		return Plan{}, fmt.Errorf("source list: %w", err)
	}
	ledger, err := a.Store.Versions(ctx)
	if err != nil {
		return Plan{}, fmt.Errorf("load ledger: %w", err)
	}

	var plan Plan
	listed := make(map[string]bool, len(entries))
	for _, e := range entries {
		listed[e.ID] = true
		if ledger[e.ID] != e.Version { // new or changed
			plan.Ingest = append(plan.Ingest, e)
		}
	}
	for docID := range ledger {
		if !listed[docID] {
			plan.Purge = append(plan.Purge, docID)
		}
	}

	sort.Slice(plan.Ingest, func(i, j int) bool { return plan.Ingest[i].ID < plan.Ingest[j].ID })
	sort.Strings(plan.Purge)
	return plan, nil
}

// ReadResult carries a document's extracted text and format back to the
// workflow. Only the extracted text crosses into workflow history — never the
// raw (possibly binary, possibly large) source bytes.
type ReadResult struct {
	Text      string
	Format    string
	Supported bool
}

// FetchAndExtract fetches a document from the Source and extracts plain text,
// routed by content type. Binary or unknown formats come back with
// Supported=false so the workflow can record them as "unsupported" instead of
// embedding garbage.
func (a *Activities) FetchAndExtract(ctx context.Context, docID string) (ReadResult, error) {
	content, err := a.Source.Fetch(ctx, docID)
	if err != nil {
		return ReadResult{}, fmt.Errorf("fetch %s: %w", docID, err)
	}
	res, err := Extract(content)
	if err != nil {
		return ReadResult{}, fmt.Errorf("extract %s: %w", docID, err)
	}
	return ReadResult{Text: res.Text, Format: res.Format, Supported: res.Supported}, nil
}

// EmbedUpsertInput carries the chunks plus the document's identity (doc ID +
// version, which together key the Qdrant payload and point IDs).
type EmbedUpsertInput struct {
	DocID   string
	Version string
	Chunks  []Chunk
}

// EmbedUpsertOutput reports how many chunks were written.
type EmbedUpsertOutput struct {
	Count int
}

// EmbedAndUpsert embeds every chunk via Ollama and writes the vectors to Qdrant
// in a single activity. Combining the two steps keeps the (large) embedding
// vectors inside the activity — they never enter the workflow's event history,
// which would otherwise store them twice (as this activity's result and the
// upsert's input). This is the slow, flaky path the project exists to exercise:
// kill Ollama or Qdrant mid-run and Temporal retries it. Retries re-embed, which
// is wasted compute but safe — upsert is idempotent via deterministic point IDs.
func (a *Activities) EmbedAndUpsert(ctx context.Context, in EmbedUpsertInput) (EmbedUpsertOutput, error) {
	if len(in.Chunks) == 0 {
		return EmbedUpsertOutput{}, nil
	}
	points := make([]Point, len(in.Chunks))
	for i, ch := range in.Chunks {
		vec, err := a.Ollama.Embed(ctx, ch.Text)
		if err != nil {
			return EmbedUpsertOutput{}, fmt.Errorf("embed chunk %d: %w", ch.Index, err)
		}
		points[i] = Point{
			ID:     ch.PointID(in.DocID, in.Version),
			Vector: vec,
			Payload: map[string]any{
				"doc_id":  in.DocID,
				"version": in.Version,
				"index":   ch.Index,
				"text":    ch.Text,
			},
		}
		activity.RecordHeartbeat(ctx, i+1) // resumable progress if retried
	}

	if err := a.Qdrant.EnsureCollection(ctx, len(points[0].Vector)); err != nil {
		return EmbedUpsertOutput{}, fmt.Errorf("ensure collection: %w", err)
	}
	if err := a.Qdrant.Upsert(ctx, points); err != nil {
		return EmbedUpsertOutput{}, fmt.Errorf("upsert: %w", err)
	}
	return EmbedUpsertOutput{Count: len(points)}, nil
}

// CleanupInput identifies the document and the version to keep.
type CleanupInput struct {
	DocID   string
	Version string
}

// DeleteStaleChunks removes Qdrant points left by previous versions of a
// document — every point for this doc ID whose version differs from the
// just-written one.
func (a *Activities) DeleteStaleChunks(ctx context.Context, in CleanupInput) error {
	return a.Qdrant.DeleteStale(ctx, in.DocID, in.Version)
}

// PurgeDocument removes a document entirely: all its Qdrant points and its ledger
// row. Called when the Source no longer lists it.
func (a *Activities) PurgeDocument(ctx context.Context, docID string) error {
	if err := a.Qdrant.DeleteDocument(ctx, docID); err != nil {
		return fmt.Errorf("purge vectors %s: %w", docID, err)
	}
	if err := a.Store.DeleteRecord(ctx, docID); err != nil {
		return fmt.Errorf("purge ledger %s: %w", docID, err)
	}
	return nil
}

// RecordIngestion writes the Postgres ledger row.
func (a *Activities) RecordIngestion(ctx context.Context, r FileRecord) error {
	return a.Store.RecordIngestion(ctx, r)
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
