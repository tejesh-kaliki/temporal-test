package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// QdrantClient is a minimal REST client for the operations this project needs:
// ensure a collection exists, upsert points, and search. Keeping it REST avoids
// pulling the gRPC client and its deps into a test project.
type QdrantClient struct {
	baseURL    string
	collection string
	http       *http.Client
}

func NewQdrantClient(baseURL, collection string, hc *http.Client) *QdrantClient {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &QdrantClient{baseURL: baseURL, collection: collection, http: hc}
}

// EnsureCollection creates the collection with the given vector size if it does
// not already exist. Qdrant's PUT /collections is not idempotent (it errors if
// the collection exists), so we check first.
func (c *QdrantClient) EnsureCollection(ctx context.Context, vectorSize int) error {
	exists, err := c.collectionExists(ctx)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	body := map[string]any{
		"vectors": map[string]any{"size": vectorSize, "distance": "Cosine"},
	}
	return c.do(ctx, http.MethodPut, "/collections/"+c.collection, body, nil)
}

func (c *QdrantClient) collectionExists(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/collections/"+c.collection, nil)
	if err != nil {
		return false, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("qdrant get collection: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK, nil
}

// Point is a vector plus its retrieval payload.
type Point struct {
	ID      string         `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

// Upsert writes points into the collection (create-or-replace by ID).
func (c *QdrantClient) Upsert(ctx context.Context, points []Point) error {
	if len(points) == 0 {
		return nil
	}
	body := map[string]any{"points": points}
	return c.do(ctx, http.MethodPut, "/collections/"+c.collection+"/points?wait=true", body, nil)
}

// DeleteStale removes every point for a document whose payload version differs
// from keepVersion — i.e. chunks left behind by a previous version. Uses Qdrant's
// delete-by-filter so it is a single call regardless of chunk count.
func (c *QdrantClient) DeleteStale(ctx context.Context, docID, keepVersion string) error {
	return c.deleteByFilter(ctx, map[string]any{
		"must": []any{
			map[string]any{"key": "doc_id", "match": map[string]any{"value": docID}},
		},
		"must_not": []any{
			map[string]any{"key": "version", "match": map[string]any{"value": keepVersion}},
		},
	})
}

// DeleteDocument removes every point for a document, regardless of version — used
// to purge a document the Source no longer lists.
func (c *QdrantClient) DeleteDocument(ctx context.Context, docID string) error {
	return c.deleteByFilter(ctx, map[string]any{
		"must": []any{
			map[string]any{"key": "doc_id", "match": map[string]any{"value": docID}},
		},
	})
}

func (c *QdrantClient) deleteByFilter(ctx context.Context, filter map[string]any) error {
	body := map[string]any{"filter": filter}
	return c.do(ctx, http.MethodPost, "/collections/"+c.collection+"/points/delete?wait=true", body, nil)
}

// SearchHit is one nearest-neighbour result.
type SearchHit struct {
	Score   float32        `json:"score"`
	Payload map[string]any `json:"payload"`
}

type searchResponse struct {
	Result []SearchHit `json:"result"`
}

// Search returns the top-k nearest neighbours to the query vector.
func (c *QdrantClient) Search(ctx context.Context, vector []float32, limit int) ([]SearchHit, error) {
	body := map[string]any{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
	}
	var out searchResponse
	if err := c.do(ctx, http.MethodPost, "/collections/"+c.collection+"/points/search", body, &out); err != nil {
		return nil, err
	}
	return out.Result, nil
}

func (c *QdrantClient) do(ctx context.Context, method, path string, body, out any) error {
	var reader *bytes.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(buf)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("qdrant %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("qdrant %s %s: status %d", method, path, resp.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("qdrant %s %s: decode: %w", method, path, err)
		}
	}
	return nil
}
