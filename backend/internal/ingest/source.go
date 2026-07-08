package ingest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Source is the external document source the pipeline syncs against — a local
// folder today, but the reconcile loop depends only on this interface, so an S3
// bucket, a CMS, or an HTTP API can be dropped in later without touching the
// workflow. Whatever List returns is treated as the desired state: any document
// previously ingested but no longer listed is considered deleted and purged.
type Source interface {
	// List returns every document the source currently holds. Each Entry carries
	// a stable ID and an opaque Version change-token; the reconcile loop re-ingests
	// a document only when its Version differs from what was last ingested.
	List(ctx context.Context) ([]Entry, error)
	// Fetch returns the raw bytes for the document with the given ID.
	Fetch(ctx context.Context, id string) ([]byte, error)
}

// Entry is one document in a Source's listing.
type Entry struct {
	// ID is the document's stable identity (for FolderSource, its path).
	ID string
	// Version changes whenever the content changes. A real source would return an
	// ETag / Last-Modified; FolderSource uses the content hash so edits — and only
	// edits — trigger re-ingestion.
	Version string
}

// FolderSource is a Source backed by a local directory. Each regular file is a
// document whose ID is its path and whose Version is the sha256 of its content.
type FolderSource struct {
	Dir string
}

var _ Source = FolderSource{}

func (s FolderSource) List(ctx context.Context) ([]Entry, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", s.Dir, err)
	}
	var out []Entry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(s.Dir, e.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			// A file that vanished mid-list is simply omitted; the next sync
			// reconciles it (as a delete if it stays gone).
			continue
		}
		out = append(out, Entry{ID: path, Version: hashBytes(content)})
	}
	return out, nil
}

func (s FolderSource) Fetch(ctx context.Context, id string) ([]byte, error) {
	content, err := os.ReadFile(id)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", id, err)
	}
	return content, nil
}
