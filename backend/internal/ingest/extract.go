package ingest

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/ledongthuc/pdf"
)

// ExtractResult is what an extractor returns: the plain text plus the detected
// format and whether the format is supported at all. Unsupported files are
// recorded (status "unsupported") and skipped rather than embedded as garbage.
type ExtractResult struct {
	Text      string
	Format    string
	Supported bool
}

// textLikeMIMEs are non-"text/*" types we still treat as plain text (their bytes
// are directly meaningful). Anything matching "text/*" is handled by prefix.
var textLikeMIMEs = map[string]bool{
	"application/json":       true,
	"application/xml":        true,
	"application/javascript": true,
	"application/x-yaml":     true,
	"application/toml":       true,
	"application/csv":        true,
}

// Extract turns raw file bytes into plain text, routing by detected content
// type. Detection uses content sniffing (not just the extension), so a
// mislabelled file is still handled correctly. Unknown/binary formats return
// Supported=false with no error — that is an expected outcome, not a failure.
func Extract(content []byte) (ExtractResult, error) {
	mtype := mimetype.Detect(content)
	mime := mtype.String()
	// mime carries a charset suffix for text (e.g. "text/plain; charset=utf-8").
	base, _, _ := strings.Cut(mime, ";")
	base = strings.TrimSpace(base)

	switch {
	case strings.HasPrefix(base, "text/") || textLikeMIMEs[base]:
		return ExtractResult{Text: string(content), Format: base, Supported: true}, nil

	case base == "application/pdf":
		text, err := extractPDF(content)
		if err != nil {
			return ExtractResult{}, fmt.Errorf("extract pdf: %w", err)
		}
		return ExtractResult{Text: text, Format: base, Supported: true}, nil

	case base == "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		text, err := extractDocx(content)
		if err != nil {
			return ExtractResult{}, fmt.Errorf("extract docx: %w", err)
		}
		return ExtractResult{Text: text, Format: "docx", Supported: true}, nil

	case base == "application/epub+zip":
		text, err := extractEpub(content)
		if err != nil {
			return ExtractResult{}, fmt.Errorf("extract epub: %w", err)
		}
		return ExtractResult{Text: text, Format: "epub", Supported: true}, nil

	default:
		return ExtractResult{Format: base, Supported: false}, nil
	}
}

// extractPDF pulls the text layer from a PDF. Scanned PDFs with no text layer
// return empty text (they need OCR — intentionally out of scope here).
func extractPDF(content []byte) (string, error) {
	r, err := pdf.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", err
	}
	br, err := r.GetPlainText()
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if _, err := io.Copy(&buf, br); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// extractDocx reads word/document.xml from the .docx zip and concatenates its
// text runs (<w:t>), inserting newlines at paragraph boundaries (<w:p>).
func extractDocx(content []byte) (string, error) {
	zr, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return "", err
	}
	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer func() { _ = rc.Close() }()
		return docxXMLText(rc)
	}
	return "", fmt.Errorf("docx: word/document.xml not found")
}

func docxXMLText(r io.Reader) (string, error) {
	dec := xml.NewDecoder(r)
	var buf strings.Builder
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.CharData:
			buf.Write(t)
		case xml.EndElement:
			// A closing paragraph tag becomes a line break so words from
			// adjacent paragraphs don't run together.
			if t.Name.Local == "p" {
				buf.WriteByte('\n')
			}
		}
	}
	return buf.String(), nil
}
