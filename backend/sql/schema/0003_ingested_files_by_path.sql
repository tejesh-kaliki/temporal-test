-- +goose Up
-- A file's identity is its PATH, not its content. Re-key the ledger on path so an
-- edited file updates its existing row (new hash) instead of inserting a second
-- row and orphaning the old one. hash becomes the "current version" column.
DELETE FROM ingested_files a
USING ingested_files b
WHERE a.path = b.path AND a.ingested_at < b.ingested_at;

ALTER TABLE ingested_files DROP CONSTRAINT ingested_files_pkey;
ALTER TABLE ingested_files ADD CONSTRAINT ingested_files_pkey PRIMARY KEY (path);

-- +goose Down
ALTER TABLE ingested_files DROP CONSTRAINT ingested_files_pkey;
ALTER TABLE ingested_files ADD CONSTRAINT ingested_files_pkey PRIMARY KEY (hash);
