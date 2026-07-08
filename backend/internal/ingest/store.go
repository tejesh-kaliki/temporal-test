package ingest

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists ingestion metadata (one row per document, keyed by the Source's
// document ID). Vectors live in Qdrant; this table is the human-facing ledger of
// what has been ingested, at which version, and its status.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// FileRecord mirrors a row of ingested_files.
type FileRecord struct {
	DocID      string
	Version    string
	ChunkCount int
	Status     string
	IngestedAt time.Time
}

// RecordIngestion upserts the ledger row for a document, keyed by doc ID. A
// changed document (same ID, new version) updates its existing row.
func (s *Store) RecordIngestion(ctx context.Context, r FileRecord) error {
	const q = `
		INSERT INTO ingested_files (doc_id, version, chunk_count, status, ingested_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (doc_id) DO UPDATE
		SET version = EXCLUDED.version,
		    chunk_count = EXCLUDED.chunk_count,
		    status = EXCLUDED.status,
		    ingested_at = now()`
	_, err := s.pool.Exec(ctx, q, r.DocID, r.Version, r.ChunkCount, r.Status)
	return err
}

// DeleteRecord removes a document's ledger row (used when the Source no longer
// lists it).
func (s *Store) DeleteRecord(ctx context.Context, docID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM ingested_files WHERE doc_id = $1`, docID)
	return err
}

// Versions returns doc_id→version for every ledger row. The reconcile loop uses
// it to decide, per Source entry, whether the document is new, changed, or
// unchanged — and which ledger docs are no longer listed (deletions).
func (s *Store) Versions(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT doc_id, version FROM ingested_files`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	versions := make(map[string]string)
	for rows.Next() {
		var docID, version string
		if err := rows.Scan(&docID, &version); err != nil {
			return nil, err
		}
		versions[docID] = version
	}
	return versions, rows.Err()
}
