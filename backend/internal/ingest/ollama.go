package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// OllamaClient talks to a local Ollama instance over its REST API. It covers the
// two calls this project needs: embeddings (ingestion + query) and generation
// (the ask endpoint).
type OllamaClient struct {
	baseURL    string
	embedModel string
	genModel   string
	http       *http.Client
}

func NewOllamaClient(baseURL, embedModel, genModel string, hc *http.Client) *OllamaClient {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &OllamaClient{baseURL: baseURL, embedModel: embedModel, genModel: genModel, http: hc}
}

type embedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type embedResponse struct {
	Embedding []float32 `json:"embedding"`
}

// Embed returns the embedding vector for a single piece of text.
func (c *OllamaClient) Embed(ctx context.Context, text string) ([]float32, error) {
	var out embedResponse
	if err := c.post(ctx, "/api/embeddings", embedRequest{Model: c.embedModel, Prompt: text}, &out); err != nil {
		return nil, err
	}
	if len(out.Embedding) == 0 {
		return nil, fmt.Errorf("ollama: empty embedding (is model %q pulled?)", c.embedModel)
	}
	return out.Embedding, nil
}

type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type generateResponse struct {
	Response string `json:"response"`
}

// Generate runs a one-shot (non-streaming) completion against the generation model.
func (c *OllamaClient) Generate(ctx context.Context, prompt string) (string, error) {
	var out generateResponse
	if err := c.post(ctx, "/api/generate", generateRequest{Model: c.genModel, Prompt: prompt, Stream: false}, &out); err != nil {
		return "", err
	}
	return out.Response, nil
}

func (c *OllamaClient) post(ctx context.Context, path string, body, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("ollama %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama %s: status %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("ollama %s: decode: %w", path, err)
	}
	return nil
}
