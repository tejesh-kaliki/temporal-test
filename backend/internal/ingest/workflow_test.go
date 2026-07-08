package ingest

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/testsuite"
)

// sampleEntry is a document the mocked Source would return.
var sampleEntry = Entry{ID: "/tmp/doc.txt", Version: "v1"}

// TestIngestDocumentWorkflow exercises the happy path with all activities mocked,
// so it runs with no Temporal server, no Ollama, and no Qdrant. It shows the
// shape of a workflow unit test: register mocks, execute, assert on the result.
func TestIngestDocumentWorkflow(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()

	var a *Activities
	env.OnActivity(a.FetchAndExtract, mockAny, mockAny).Return(
		ReadResult{Text: "hello world", Format: "text/plain", Supported: true}, nil)
	env.OnActivity(a.EmbedAndUpsert, mockAny, mockAny).Return(
		EmbedUpsertOutput{Count: 1}, nil)
	env.OnActivity(a.DeleteStaleChunks, mockAny, mockAny).Return(nil)
	env.OnActivity(a.RecordIngestion, mockAny, mockAny).Return(nil)

	env.ExecuteWorkflow(IngestDocumentWorkflow, sampleEntry)

	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}
	var result IngestResult
	if err := env.GetWorkflowResult(&result); err != nil {
		t.Fatalf("get result: %v", err)
	}
	if result.Status != "ingested" || result.DocID != sampleEntry.ID {
		t.Fatalf("unexpected result: %+v", result)
	}
}

// TestIngestDocumentWorkflow_EmptyFile verifies a whitespace-only document is
// recorded as "empty" and never reaches embedding/upsert.
func TestIngestDocumentWorkflow_EmptyFile(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()

	var a *Activities
	env.OnActivity(a.FetchAndExtract, mockAny, mockAny).Return(
		ReadResult{Text: "   \n\t ", Format: "text/plain", Supported: true}, nil)
	env.OnActivity(a.RecordIngestion, mockAny, mockAny).Return(nil)

	env.ExecuteWorkflow(IngestDocumentWorkflow, sampleEntry)

	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}
	var result IngestResult
	if err := env.GetWorkflowResult(&result); err != nil {
		t.Fatalf("get result: %v", err)
	}
	if result.Status != "empty" {
		t.Fatalf("expected empty status, got %+v", result)
	}
}

// TestIngestDocumentWorkflow_Unsupported verifies a binary/unknown format is
// recorded as "unsupported" and never reaches embedding.
func TestIngestDocumentWorkflow_Unsupported(t *testing.T) {
	var ts testsuite.WorkflowTestSuite
	env := ts.NewTestWorkflowEnvironment()

	var a *Activities
	env.OnActivity(a.FetchAndExtract, mockAny, mockAny).Return(
		ReadResult{Format: "application/octet-stream", Supported: false}, nil)
	env.OnActivity(a.RecordIngestion, mockAny, mockAny).Return(nil)

	env.ExecuteWorkflow(IngestDocumentWorkflow, sampleEntry)

	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}
	var result IngestResult
	if err := env.GetWorkflowResult(&result); err != nil {
		t.Fatalf("get result: %v", err)
	}
	if result.Status != "unsupported" {
		t.Fatalf("expected unsupported status, got %+v", result)
	}
}

func TestExtract(t *testing.T) {
	// Plain text passes through.
	res, err := Extract([]byte("hello world"))
	if err != nil {
		t.Fatalf("extract text: %v", err)
	}
	if !res.Supported || res.Text != "hello world" {
		t.Fatalf("unexpected text result: %+v", res)
	}

	// JSON is treated as text-like.
	res, err = Extract([]byte(`{"key": "value"}`))
	if err != nil {
		t.Fatalf("extract json: %v", err)
	}
	if !res.Supported {
		t.Fatalf("json should be supported: %+v", res)
	}

	// A PNG signature is detected as unsupported (no error).
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	res, err = Extract(png)
	if err != nil {
		t.Fatalf("extract png: %v", err)
	}
	if res.Supported {
		t.Fatalf("png should be unsupported: %+v", res)
	}
}

func TestChunkText(t *testing.T) {
	// Empty / whitespace yields no chunks.
	if got := ChunkText("   "); got != nil {
		t.Fatalf("expected nil for blank text, got %v", got)
	}

	// Short text is a single chunk.
	one := ChunkText("hello")
	if len(one) != 1 || one[0].Text != "hello" {
		t.Fatalf("unexpected single chunk: %+v", one)
	}

	// Point IDs are deterministic and differ per index and per version.
	id0 := (Chunk{Index: 0}).PointID("doc", "v1")
	if id0 != (Chunk{Index: 0}).PointID("doc", "v1") {
		t.Fatal("point ID not deterministic")
	}
	if id0 == (Chunk{Index: 1}).PointID("doc", "v1") {
		t.Fatal("point IDs collide across indices")
	}
	if id0 == (Chunk{Index: 0}).PointID("doc", "v2") {
		t.Fatal("point IDs collide across versions")
	}
}

// mockAny matches any argument in activity mocks. Kept as a package var so the
// intent ("any value") reads clearly at each call site.
var mockAny = mock.Anything
