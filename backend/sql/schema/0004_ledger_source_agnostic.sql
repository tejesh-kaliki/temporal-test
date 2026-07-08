-- +goose Up
-- The ledger is now keyed by a Source's opaque document ID (a path for the
-- folder source, but could be an S3 key or CMS ID later) with an opaque version
-- token. Rename the folder-specific columns to reflect that.
ALTER TABLE ingested_files RENAME COLUMN path TO doc_id;
ALTER TABLE ingested_files RENAME COLUMN hash TO version;

-- +goose Down
ALTER TABLE ingested_files RENAME COLUMN doc_id TO path;
ALTER TABLE ingested_files RENAME COLUMN version TO hash;
