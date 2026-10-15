package testutil

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// WriteFB2 writes a minimal UTF-8 FB2 book; a different title gives a different file hash.
func WriteFB2(t *testing.T, dir, title, author string) string {
	t.Helper()

	content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<FictionBook xmlns="http://www.gribuser.ru/xml/fictionbook/2.0"><description><title-info>
<genre>prose</genre><author><first-name>%s</first-name></author><book-title>%s</book-title><keywords>fiction, war</keywords><lang>ru</lang>
</title-info></description><body><section><p>text</p></section></body></FictionBook>`, author, title)
	path := filepath.Join(dir, strings.ReplaceAll(title, "/", "_")+".fb2")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// WriteEPUB writes a minimal EPUB with a PNG cover.
func WriteEPUB(t *testing.T, dir, title, author string) string {
	t.Helper()

	var cover, buf bytes.Buffer
	require.NoError(t, png.Encode(&cover, image.NewRGBA(image.Rect(0, 0, 1, 1))))
	files := []struct{ name, body string }{
		{"mimetype", "application/epub+zip"},
		{"META-INF/container.xml", `<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/book.opf"/></rootfiles></container>`},
		{"OEBPS/book.opf", fmt.Sprintf(`<package xmlns="http://www.idpf.org/2007/opf" xmlns:dc="http://purl.org/dc/elements/1.1/"><metadata>
<dc:title>%s</dc:title><dc:creator>%s</dc:creator><dc:language>rus</dc:language><dc:subject>fiction</dc:subject>
<dc:date>2012-05-01</dc:date><dc:description>&lt;p&gt;Some &lt;b&gt;text&lt;/b&gt;&lt;/p&gt;</dc:description>
<dc:identifier xmlns:opf="http://www.idpf.org/2007/opf" opf:scheme="ISBN">978-5-389-09940-1</dc:identifier>
<dc:identifier xmlns:opf="http://www.idpf.org/2007/opf" opf:scheme="litres">42</dc:identifier>
<dc:identifier xmlns:opf="http://www.idpf.org/2007/opf" opf:scheme="UUID">urn:uuid:0b2f5a3e</dc:identifier>
<meta name="cover" content="cover"/></metadata>
<manifest><item id="cover" href="cover.png" media-type="image/png"/></manifest></package>`, title, author)},
		{"OEBPS/cover.png", cover.String()},
	}
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(f.name)
		require.NoError(t, err)
		_, err = w.Write([]byte(f.body))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	path := filepath.Join(dir, strings.ReplaceAll(title, "/", "_")+".epub")
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o600))
	return path
}

// PNG returns a blank square PNG; a different size gives different bytes.
func PNG(t *testing.T, size int) []byte {
	t.Helper()

	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, size, size))))
	return buf.Bytes()
}
