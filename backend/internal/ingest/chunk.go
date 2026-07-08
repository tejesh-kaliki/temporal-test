package ingest

import (
	"strings"

	"github.com/google/uuid"
)

// chunkNamespace makes per-chunk point IDs deterministic: the same file hash and
// chunk index always map to the same Qdrant point, so re-ingesting a file
// overwrites its points instead of duplicating them.
var chunkNamespace = uuid.MustParse("6ba7b811-9dad-11d1-80b4-00c04fd430c8")

const (
	// defaultChunkSize is the target chunk length in characters. Kept simple
	// (character windows, not token-aware) — this is a Temporal learning project,
	// not a retrieval-quality one.
	defaultChunkSize = 1000
	// defaultOverlap carries context across chunk boundaries.
	defaultOverlap = 200
)

// Chunk is one slice of a document destined for its own embedding.
type Chunk struct {
	Index int    `json:"index"`
	Text  string `json:"text"`
}

// PointID returns the deterministic Qdrant point ID for this chunk of a specific
// version of a document. Including the version means a new version writes new
// point IDs (leaving the old ones to be cleaned up), while re-running the same
// version overwrites in place.
func (c Chunk) PointID(docID, version string) string {
	return uuid.NewSHA1(chunkNamespace, []byte(docID+":"+version+":"+itoa(c.Index))).String()
}

// ChunkText splits text into overlapping character windows. Deterministic and
// dependency-free so it is safe to call from either a workflow or an activity.
func ChunkText(text string) []Chunk {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	runes := []rune(text)
	var chunks []Chunk
	step := defaultChunkSize - defaultOverlap
	for start, idx := 0, 0; start < len(runes); start, idx = start+step, idx+1 {
		end := min(start+defaultChunkSize, len(runes))
		chunks = append(chunks, Chunk{Index: idx, Text: string(runes[start:end])})
		if end == len(runes) {
			break
		}
	}
	return chunks
}

// itoa avoids pulling strconv into a hot deterministic path for tiny ints.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
