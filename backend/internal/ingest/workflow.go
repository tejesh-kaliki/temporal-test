package ingest

import (
	"time"

	enums "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// Workflow names and the Schedule ID. Registering by explicit name keeps the
// client (server) decoupled from the worker's Go symbols.
const (
	SyncCatalogWorkflowName    = "SyncCatalogWorkflow"
	IngestDocumentWorkflowName = "IngestDocumentWorkflow"
	PurgeDocumentWorkflowName  = "PurgeDocumentWorkflow"
	ScheduleID                 = "ingest-poll"
)

// IngestResult is the ingest workflow's return value (visible in the Web UI).
type IngestResult struct {
	DocID   string
	Version string
	Chunks  int
	Status  string
}

// IngestDocumentWorkflow orchestrates the pipeline for one document: fetch →
// extract → chunk → embed+upsert → clean up old version → record. Only
// orchestration lives here; every side effect is an activity, so this function
// stays deterministic and replay-safe.
func IngestDocumentWorkflow(ctx workflow.Context, entry Entry) (IngestResult, error) {
	// Embedding is slow and flaky, so give activities room and retry generously.
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    0, // unlimited — durability is the whole point
		},
	})

	var a *Activities // nil receiver: used only to reference activity methods

	var read ReadResult
	if err := workflow.ExecuteActivity(ctx, a.FetchAndExtract, entry.ID).Get(ctx, &read); err != nil {
		return IngestResult{}, err
	}

	// Unsupported formats (images, binaries, scanned PDFs with no text layer) are
	// recorded and skipped rather than embedded as garbage.
	if !read.Supported {
		_ = workflow.ExecuteActivity(ctx, a.RecordIngestion, FileRecord{
			DocID: entry.ID, Version: entry.Version, ChunkCount: 0, Status: "unsupported",
		}).Get(ctx, nil)
		return IngestResult{DocID: entry.ID, Version: entry.Version, Status: "unsupported"}, nil
	}

	// Chunking is deterministic, so it runs inline in the workflow.
	chunks := ChunkText(read.Text)
	if len(chunks) == 0 {
		_ = workflow.ExecuteActivity(ctx, a.RecordIngestion, FileRecord{
			DocID: entry.ID, Version: entry.Version, ChunkCount: 0, Status: "empty",
		}).Get(ctx, nil)
		return IngestResult{DocID: entry.ID, Version: entry.Version, Status: "empty"}, nil
	}

	// Embed + upsert in one activity so the vectors stay out of workflow history.
	var upserted EmbedUpsertOutput
	if err := workflow.ExecuteActivity(ctx, a.EmbedAndUpsert, EmbedUpsertInput{
		DocID: entry.ID, Version: entry.Version, Chunks: chunks,
	}).Get(ctx, &upserted); err != nil {
		return IngestResult{}, err
	}

	// Remove chunks left by a previous version of this document (same doc ID,
	// different version) so an edit doesn't leave orphaned vectors behind.
	if err := workflow.ExecuteActivity(ctx, a.DeleteStaleChunks, CleanupInput{
		DocID: entry.ID, Version: entry.Version,
	}).Get(ctx, nil); err != nil {
		return IngestResult{}, err
	}

	if err := workflow.ExecuteActivity(ctx, a.RecordIngestion, FileRecord{
		DocID: entry.ID, Version: entry.Version, ChunkCount: len(chunks), Status: "ingested",
	}).Get(ctx, nil); err != nil {
		return IngestResult{}, err
	}

	return IngestResult{DocID: entry.ID, Version: entry.Version, Chunks: len(chunks), Status: "ingested"}, nil
}

// PurgeDocumentWorkflow removes a document that the Source no longer lists: its
// Qdrant vectors and its ledger row.
func PurgeDocumentWorkflow(ctx workflow.Context, docID string) error {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
		},
	})
	var a *Activities
	return workflow.ExecuteActivity(ctx, a.PurgeDocument, docID).Get(ctx, nil)
}

// SyncCatalogWorkflow is the Schedule target and the reconcile loop. Each tick it
// diffs the Source against the ledger, then starts one child per change: an
// IngestDocumentWorkflow for new/changed documents, a PurgeDocumentWorkflow for
// documents the Source no longer lists. Children are keyed by doc ID, so an edit
// re-runs the same logical ingest workflow — content identity lives in the
// payload, not the workflow ID.
func SyncCatalogWorkflow(ctx workflow.Context) error {
	ao := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: time.Minute,
	})

	var a *Activities
	var plan Plan
	if err := workflow.ExecuteActivity(ao, a.Reconcile).Get(ao, &plan); err != nil {
		return err
	}

	log := workflow.GetLogger(ctx)
	for _, entry := range plan.Ingest {
		startChild(ctx, log, "ingest:"+entry.ID, IngestDocumentWorkflow, entry, "ingest", entry.ID)
	}
	for _, docID := range plan.Purge {
		startChild(ctx, log, "purge:"+docID, PurgeDocumentWorkflow, docID, "purge", docID)
	}
	return nil
}

// startChild launches a child workflow and waits only for it to *start* (or be
// rejected because one is already running), then moves on — the work runs
// independently of this sync tick.
func startChild(ctx workflow.Context, log interface {
	Info(string, ...any)
}, workflowID string, wf any, arg any, action, docID string) {
	childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
		WorkflowID: workflowID,
		// AllowDuplicate: an edited/re-deleted document must be able to re-run.
		// The reconcile already filtered out unchanged documents, so this doesn't
		// cause redundant work; a still-running prior attempt fails to start and is
		// skipped until the next tick.
		WorkflowIDReusePolicy: enums.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE,
		ParentClosePolicy:     enums.PARENT_CLOSE_POLICY_ABANDON,
	})
	child := workflow.ExecuteChildWorkflow(childCtx, wf, arg)
	var exec workflow.Execution
	if err := child.GetChildWorkflowExecution().Get(ctx, &exec); err != nil {
		log.Info("skip "+action+" (already running)", "docID", docID, "err", err)
		return
	}
	log.Info("started "+action, "docID", docID, "workflowID", exec.ID)
}
