package ingest

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"

	"golang.org/x/net/html"
)

// epubContainerFiles opens content as a zip and, if it carries the mandatory
// META-INF/container.xml OCF marker, returns its file index. mimetype's Epub
// magic requires the "mimetype" entry to be the literal first, stored zip
// entry (the strict spec rule) and misdetects any EPUB that doesn't comply —
// which in practice includes real, professionally-distributed EPUBs that have
// been round-tripped through Calibre or similar tools and end up with
// META-INF/ entries reordered ahead of it. container.xml's presence is the
// same spec-mandated signal without that ordering sensitivity, so this is used
// as the actual detection check (see Extract's default case), with the
// MIME-based route left only as a fast, common-case hint.
func epubContainerFiles(content []byte) (map[string]*zip.File, bool) {
	zr, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, false
	}
	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[f.Name] = f
	}
	if _, ok := files["META-INF/container.xml"]; !ok {
		return nil, false
	}
	return files, true
}

// extractEpubFiles extracts spine-ordered plain text from an already-opened
// EPUB's file index. META-INF/container.xml points at an OPF package file
// whose <manifest> lists every file and whose <spine> gives the reading order
// as a list of manifest IDs. This mirrors extractDocx's approach (zip + XML,
// stdlib only) rather than pulling in a full EPUB library, since parsing
// container.xml + the OPF manifest/spine is the same complexity class as
// docx's document.xml. Token-based decoding (like docxXMLText) rather than
// xml.Unmarshal sidesteps the default-namespace attributes OPF/container.xml
// declare, which struct-tag matching would otherwise need to account for.
func extractEpubFiles(files map[string]*zip.File) (string, error) {
	opfPath, err := epubOPFPath(files)
	if err != nil {
		return "", err
	}
	manifest, spine, err := epubReadOPF(files, opfPath)
	if err != nil {
		return "", err
	}

	baseDir := path.Dir(opfPath)
	var buf strings.Builder
	for _, idref := range spine {
		href, ok := manifest[idref]
		if !ok {
			continue // itemref referencing a missing manifest entry: skip rather than fail the whole book
		}
		docPath := path.Join(baseDir, href)
		f, ok := files[docPath]
		if !ok {
			continue
		}
		text, err := epubDocText(f)
		if err != nil {
			return "", fmt.Errorf("epub: read %s: %w", docPath, err)
		}
		if text = strings.TrimSpace(text); text != "" {
			buf.WriteString(text)
			buf.WriteString("\n\n")
		}
	}
	return buf.String(), nil
}

// epubOPFPath reads META-INF/container.xml and returns the path of the first
// rootfile — the OPF package document. EPUBs can list more than one rootfile
// (for alternate renditions); the first is the primary one.
func epubOPFPath(files map[string]*zip.File) (string, error) {
	f, ok := files["META-INF/container.xml"]
	if !ok {
		return "", fmt.Errorf("epub: META-INF/container.xml not found")
	}
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()

	dec := xml.NewDecoder(rc)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("epub: parse container.xml: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok || se.Name.Local != "rootfile" {
			continue
		}
		for _, attr := range se.Attr {
			if attr.Name.Local == "full-path" {
				return attr.Value, nil
			}
		}
	}
	return "", fmt.Errorf("epub: no rootfile in container.xml")
}

// epubReadOPF parses the OPF package document into a manifest ID->href map and
// the spine's ordered list of manifest IDs.
func epubReadOPF(files map[string]*zip.File, opfPath string) (manifest map[string]string, spine []string, err error) {
	f, ok := files[opfPath]
	if !ok {
		return nil, nil, fmt.Errorf("epub: opf %s not found", opfPath)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rc.Close() }()

	manifest = make(map[string]string)
	dec := xml.NewDecoder(rc)
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("epub: parse opf: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "item":
			var id, href string
			for _, attr := range se.Attr {
				switch attr.Name.Local {
				case "id":
					id = attr.Value
				case "href":
					href = attr.Value
				}
			}
			if id != "" && href != "" {
				manifest[id] = href
			}
		case "itemref":
			for _, attr := range se.Attr {
				if attr.Name.Local == "idref" {
					spine = append(spine, attr.Value)
				}
			}
		}
	}
	if len(spine) == 0 {
		return nil, nil, fmt.Errorf("epub: no itemref entries in spine")
	}
	return manifest, spine, nil
}

// blockBreakTags are HTML elements whose boundaries become a line break in the
// extracted text, so words from adjacent block elements don't run together.
var blockBreakTags = map[string]bool{
	"p": true, "div": true, "br": true, "li": true, "tr": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
}

// epubDocText strips HTML markup from one EPUB content document, returning its
// plain text. Content inside <script>/<style> is dropped entirely.
func epubDocText(f *zip.File) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()

	tok := html.NewTokenizer(rc)
	var buf strings.Builder
	var skipDepth int
	for {
		switch tok.Next() {
		case html.ErrorToken:
			if err := tok.Err(); err != io.EOF && err != nil {
				return "", err
			}
			return buf.String(), nil

		case html.TextToken:
			if skipDepth == 0 {
				buf.Write(tok.Text())
			}

		case html.StartTagToken:
			name, _ := tok.TagName()
			tag := string(name)
			if tag == "script" || tag == "style" {
				skipDepth++
				continue
			}
			if blockBreakTags[tag] {
				buf.WriteByte('\n')
			}

		case html.SelfClosingTagToken:
			name, _ := tok.TagName()
			if blockBreakTags[string(name)] {
				buf.WriteByte('\n')
			}

		case html.EndTagToken:
			name, _ := tok.TagName()
			tag := string(name)
			if tag == "script" || tag == "style" {
				if skipDepth > 0 {
					skipDepth--
				}
				continue
			}
			if blockBreakTags[tag] {
				buf.WriteByte('\n')
			}
		}
	}
}
