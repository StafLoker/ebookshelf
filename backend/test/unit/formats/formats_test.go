package formats_test

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/encoding/charmap"

	"ebookshelf/internal/domain"
	"ebookshelf/internal/formats"
	"ebookshelf/test/testutil"
)

func TestEPUB_Read_OK(t *testing.T) {
	path := testutil.WriteEPUB(t, t.TempDir(), "Title", "Ann  Author")

	meta, err := formats.For("epub").Read(path)

	require.NoError(t, err)
	assert.Equal(t, "Title", meta.Title)
	assert.Equal(t, []domain.Author{{FirstName: "Ann", LastName: "Author"}}, meta.Authors)
	assert.Equal(t, "ru", meta.Language)
	assert.Equal(t, []string{"fiction"}, meta.Tags)
	assert.Equal(t, 2012, meta.Year)
	assert.Equal(t, []domain.Identifier{{Type: "isbn", Value: "9785389099401"}, {Type: "litres", Value: "42"}}, meta.Identifiers)
	assert.Equal(t, "Some text", meta.Description)
	assert.NotEmpty(t, meta.Cover)
}

func TestEPUB_Read_NotAZip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.epub")
	require.NoError(t, os.WriteFile(path, []byte("not a zip"), 0o600))

	_, err := formats.For("epub").Read(path)

	assert.Error(t, err)
}

func TestFB2_Read_OK(t *testing.T) {
	path := testutil.WriteFB2(t, t.TempDir(), "Title", "Ann")

	meta, err := formats.For("fb2").Read(path)

	require.NoError(t, err)
	assert.Equal(t, "Title", meta.Title)
	assert.Equal(t, []domain.Author{{LastName: "Ann"}}, meta.Authors)
	assert.Equal(t, []string{"fiction", "war"}, meta.Tags)
	assert.Equal(t, "ru", meta.Language)
}

func TestFB2_Read_Windows1251(t *testing.T) {
	encoded, err := charmap.Windows1251.NewEncoder().String(`<?xml version="1.0" encoding="windows-1251"?>
<FictionBook><description><title-info><book-title>Тихий Дон</book-title></title-info></description></FictionBook>`)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "book.fb2")
	require.NoError(t, os.WriteFile(path, []byte(encoded), 0o600))

	meta, err := formats.For("fb2").Read(path)

	require.NoError(t, err)
	assert.Equal(t, "Тихий Дон", meta.Title)
}

func TestFor_Unknown(t *testing.T) {
	assert.Nil(t, formats.For("pdf"))
}

func TestFB2_Read_NestedTextNicknameAndDateValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "book.fb2")
	require.NoError(t, os.WriteFile(path, []byte(`<?xml version="1.0" encoding="UTF-8"?>
<FictionBook><description><title-info><author><nickname>neo</nickname></author><book-title>T</book-title>
<keywords>a, b;c</keywords><annotation><p>Hello <emphasis>big</emphasis> world</p></annotation><date value="2005-01-01">1 Jan 2005</date></title-info>
<publish-info><isbn>5-93972-087-0, 5-88-1</isbn></publish-info></description></FictionBook>`), 0o600))

	meta, err := formats.For("fb2").Read(path)

	require.NoError(t, err)
	assert.Equal(t, []domain.Author{{LastName: "neo"}}, meta.Authors)
	assert.Equal(t, []string{"a", "b", "c"}, meta.Tags)
	assert.Equal(t, "Hello big world", meta.Description)
	assert.Equal(t, 2005, meta.Year)
	assert.Equal(t, []domain.Identifier{{Type: "isbn", Value: "5939720870"}}, meta.Identifiers)
}

func TestEPUB_Write_RoundTrip(t *testing.T) {
	src := testutil.WriteEPUB(t, t.TempDir(), "Title", "Ann Author")
	dst := filepath.Join(t.TempDir(), "out.epub")
	meta, err := formats.For("epub").Read(src)
	require.NoError(t, err)
	meta.Title, meta.Publisher, meta.Year = "New Title", "Press", 1999
	meta.Authors = []domain.Author{{FirstName: "Fiódor", MiddleName: "Mijáilovich", LastName: "Dostoievski"}, {FirstName: "Gabriel", LastName: "García Márquez"}}
	meta.Tags = []string{"classic", "novel"}
	meta.Identifiers = []domain.Identifier{{Type: "isbn", Value: "9785389099401"}, {Type: "doi", Value: "10.1/x"}}
	meta.Cover = testutil.PNG(t, 2)
	want := *meta

	err = formats.For("epub").Write(src, dst, meta)
	got, readErr := formats.For("epub").Read(dst)
	zr, zipErr := zip.OpenReader(dst)

	require.NoError(t, err)
	require.NoError(t, readErr)
	require.NoError(t, zipErr)
	defer func() { _ = zr.Close() }()
	assert.Equal(t, &want, got)
	assert.Equal(t, "mimetype", zr.File[0].Name)
	assert.Equal(t, zip.Store, zr.File[0].Method)
}

func TestFB2_Write_RoundTripWindows1251ToUTF8(t *testing.T) {
	encoded, err := charmap.Windows1251.NewEncoder().String(`<?xml version="1.0" encoding="windows-1251"?>
<FictionBook xmlns="http://www.gribuser.ru/xml/fictionbook/2.0"><description><title-info><genre>prose</genre><book-title>Тихий Дон</book-title><coverpage><image xmlns:l="http://www.w3.org/1999/xlink" l:href="#c.png"/></coverpage><lang>ru</lang></title-info></description><body><section><p>текст</p></section></body><binary id="c.png" content-type="image/png">AAAA</binary></FictionBook>`)
	require.NoError(t, err)
	src := filepath.Join(t.TempDir(), "book.fb2")
	dst := filepath.Join(t.TempDir(), "out.fb2")
	require.NoError(t, os.WriteFile(src, []byte(encoded), 0o600))
	meta, err := formats.For("fb2").Read(src)
	require.NoError(t, err)
	meta.Authors = []domain.Author{{FirstName: "Михаил", MiddleName: "Александрович", LastName: "Шолохов"}, {LastName: "Plato"}}
	meta.Tags = []string{"classic"}
	meta.Description, meta.Publisher, meta.Year = "Первая\nВторая", "Издательство", 1928
	meta.Identifiers = []domain.Identifier{{Type: "isbn", Value: "9785389099401"}}
	meta.Cover = testutil.PNG(t, 2)
	want := *meta

	err = formats.For("fb2").Write(src, dst, meta)
	got, readErr := formats.For("fb2").Read(dst)
	body, fileErr := os.ReadFile(dst)

	require.NoError(t, err)
	require.NoError(t, readErr)
	require.NoError(t, fileErr)
	assert.Equal(t, &want, got)
	assert.Contains(t, string(body), `encoding="utf-8"`)
	assert.True(t, utf8.Valid(body))
}

func TestEPUB_Read_RealWorldAuthorsAndDate(t *testing.T) {
	src := filepath.Join(t.TempDir(), "book.epub")
	writeZip(t, src, `<package xmlns="http://www.idpf.org/2007/opf" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:opf="http://www.idpf.org/2007/opf"><metadata>
<dc:title>T</dc:title><dc:creator opf:role="aut" opf:file-as="Garrido, Vicente &amp; Abarca, Nieves">Vicente Garrido &amp; Nieves Abarca</dc:creator>
<dc:creator>Толстой Л.Н.</dc:creator><dc:date opf:event="creation">2013-06-09</dc:date><dc:date opf:event="publication">1967-05-01</dc:date></metadata><manifest/></package>`)

	meta, err := formats.For("epub").Read(src)

	require.NoError(t, err)
	assert.Equal(t, []domain.Author{
		{FirstName: "Vicente", LastName: "Garrido"},
		{FirstName: "Nieves", LastName: "Abarca"},
		{FirstName: "Л.Н.", LastName: "Толстой"},
	}, meta.Authors)
	assert.Equal(t, 1967, meta.Year)
}

// an EPUB whose package document is opf
func writeZip(t *testing.T, path, opf string) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	zw := zip.NewWriter(f)
	for _, entry := range [][2]string{
		{"mimetype", "application/epub+zip"},
		{"META-INF/container.xml", `<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="content.opf"/></rootfiles></container>`},
		{"content.opf", opf},
	} {
		w, err := zw.Create(entry[0])
		require.NoError(t, err)
		_, err = w.Write([]byte(entry[1]))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
}
