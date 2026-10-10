package formats_test

import (
	"os"
	"path/filepath"
	"testing"

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
	assert.Equal(t, []string{"Ann Author"}, meta.Authors)
	assert.Equal(t, "ru", meta.Language)
	assert.Equal(t, []string{"fiction"}, meta.Tags)
	assert.Equal(t, 2012, meta.Year)
	assert.Equal(t, []domain.Identifier{{Type: "isbn", Value: "9785389099401"}, {Type: "litres", Value: "42"}, {Type: "isbn", Value: "5939720870"}, {Type: "isbn", Value: "978517111111X"}}, meta.Identifiers)
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
	assert.Equal(t, []string{"Ann"}, meta.Authors)
	assert.Equal(t, []string{"prose"}, meta.Tags)
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
<annotation><p>Hello <emphasis>big</emphasis> world</p></annotation><date value="2005-01-01">1 Jan 2005</date></title-info>
<publish-info><isbn>ISBN 5-93972-087-0 (hardcover), 5-88-1</isbn></publish-info></description></FictionBook>`), 0o600))

	meta, err := formats.For("fb2").Read(path)

	require.NoError(t, err)
	assert.Equal(t, []string{"neo"}, meta.Authors)
	assert.Equal(t, "Hello big world", meta.Description)
	assert.Equal(t, 2005, meta.Year)
	assert.Equal(t, []domain.Identifier{{Type: "isbn", Value: "5939720870"}}, meta.Identifiers)
}
