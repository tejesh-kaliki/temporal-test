package ingest

import (
	"archive/zip"
	"bytes"
	"os"
	"strings"
	"testing"
)

// TestExtractEpub_RealBook runs full extraction against a real, public-domain
// EPUB (Lewis Carroll's "Alice's Adventures in Wonderland" from Project
// Gutenberg, testdata/alice.epub) to verify actual extraction quality rather
// than just a synthetic mock: content sniffing detects it, chapters come out
// in spine (reading) order, and the HTML markup is fully stripped.
func TestExtractEpub_RealBook(t *testing.T) {
	content, err := os.ReadFile("testdata/alice.epub")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	res, err := Extract(content)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if !res.Supported {
		t.Fatalf("expected epub to be supported, got %+v", res)
	}
	if res.Format != "epub" {
		t.Fatalf("expected format epub, got %q", res.Format)
	}

	text := res.Text
	if strings.TrimSpace(text) == "" {
		t.Fatal("extracted text is empty")
	}

	// No HTML markup should have leaked through.
	for _, marker := range []string{"<html", "<body", "<p>", "<div", "</p>", "<!DOCTYPE"} {
		if strings.Contains(text, marker) {
			t.Fatalf("extracted text still contains HTML markup %q", marker)
		}
	}

	// Recognizable prose from the book should be present, confirming the
	// content documents were actually read and tag-stripped, not garbled.
	for _, want := range []string{"Alice", "White Rabbit", "rabbit-hole"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected extracted text to contain %q", want)
		}
	}

	// Chapter I ("Down the Rabbit-Hole") must precede Chapter II ("The Pool of
	// Tears") in the output — this is only true if the spine order was
	// respected rather than, say, files being read in arbitrary/lexical zip
	// order (which would sort "...-h-10..." before "...-h-2...").
	iIdx := strings.Index(text, "Down the Rabbit-Hole")
	iiIdx := strings.Index(text, "The Pool of Tears")
	if iIdx == -1 || iiIdx == -1 {
		t.Fatalf("expected both chapter headings present: I=%d II=%d", iIdx, iiIdx)
	}
	if iIdx > iiIdx {
		t.Fatal("chapters are out of spine order")
	}
}

// buildTestEpub assembles a minimal in-memory EPUB from the given spine-ordered
// (id, xhtml) content documents, wiring up container.xml/OPF manifest+spine by
// hand so the zip/OPF-parsing logic can be tested without network access.
func buildTestEpub(t *testing.T, docs []struct{ id, xhtml string }) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	write := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	write("META-INF/container.xml", `<?xml version="1.0"?>
<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`)

	var manifest, spine strings.Builder
	for _, d := range docs {
		manifest.WriteString(`<item id="` + d.id + `" href="` + d.id + `.xhtml" media-type="application/xhtml+xml"/>`)
		spine.WriteString(`<itemref idref="` + d.id + `"/>`)
		write("OEBPS/"+d.id+".xhtml", d.xhtml)
	}
	write("OEBPS/content.opf", `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata/>
  <manifest>`+manifest.String()+`</manifest>
  <spine>`+spine.String()+`</spine>
</package>`)

	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func TestExtractEpub_SpineOrder(t *testing.T) {
	epub := buildTestEpub(t, []struct{ id, xhtml string }{
		{"c10", "<html><body><p>tenth chapter</p></body></html>"},
		{"c2", "<html><body><p>second chapter</p></body></html>"},
	})

	text, err := extractEpub(epub)
	if err != nil {
		t.Fatalf("extractEpub: %v", err)
	}

	// The spine lists c10 before c2, so despite c2 sorting first lexically
	// (and first if walked in zip file order), "tenth" must come first.
	tenIdx := strings.Index(text, "tenth")
	secondIdx := strings.Index(text, "second")
	if tenIdx == -1 || secondIdx == -1 {
		t.Fatalf("missing expected content: %q", text)
	}
	if tenIdx > secondIdx {
		t.Fatalf("spine order not respected: %q", text)
	}
}

func TestExtractEpub_StripsScriptAndStyle(t *testing.T) {
	epub := buildTestEpub(t, []struct{ id, xhtml string }{
		{"c1", `<html><head><style>p { color: red; }</style><script>alert("hi")</script></head>` +
			`<body><p>visible text</p></body></html>`},
	})

	text, err := extractEpub(epub)
	if err != nil {
		t.Fatalf("extractEpub: %v", err)
	}
	if !strings.Contains(text, "visible text") {
		t.Fatalf("expected visible text present: %q", text)
	}
	if strings.Contains(text, "color: red") || strings.Contains(text, "alert") {
		t.Fatalf("script/style content leaked into extracted text: %q", text)
	}
}

func TestExtractEpub_MissingManifestItem(t *testing.T) {
	// A spine itemref referencing an idref absent from the manifest should be
	// skipped rather than failing the whole book.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	write := func(name, content string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	write("META-INF/container.xml", `<?xml version="1.0"?>
<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>`)
	write("OEBPS/c1.xhtml", "<html><body><p>only chapter</p></body></html>")
	write("OEBPS/content.opf", `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="2.0">
  <metadata/>
  <manifest><item id="c1" href="c1.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine><itemref idref="c1"/><itemref idref="does-not-exist"/></spine>
</package>`)
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	text, err := extractEpub(buf.Bytes())
	if err != nil {
		t.Fatalf("extractEpub should skip the dangling itemref, not fail: %v", err)
	}
	if !strings.Contains(text, "only chapter") {
		t.Fatalf("expected the valid chapter's text present: %q", text)
	}
}
