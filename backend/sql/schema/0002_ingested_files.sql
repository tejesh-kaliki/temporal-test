-- +goose Up
-- Ledger of files ingested into the RAG pipeline. One row per unique file
-- content (keyed by sha256 hash), written by the ingestion workflow. The vectors
-- themselves live in Qdrant; this table is the queryable record of what exists.
CREATE TABLE ingested_files (
    hash        TEXT PRIMARY KEY,
    path        TEXT NOT NULL,
    chunk_count INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'ingested',
    ingested_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE ingested_files;
