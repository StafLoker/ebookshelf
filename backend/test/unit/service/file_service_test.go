package service_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ebookshelf/internal/domain"
	apperrors "ebookshelf/internal/errors"
	"ebookshelf/internal/service"
)

func TestFileService_Save_OKMoveAndRemove(t *testing.T) {
	root := t.TempDir()
	files := service.NewFileService(root + "/")
	policy := domain.UploadPolicy{AllowedExts: []string{"epub"}, MaxSizeBytes: 10}

	require.NoError(t, files.Save(ctx, "tmp/a/book.EPUB", strings.NewReader("data"), policy))
	require.NoError(t, files.Move("tmp/a", "books/1"))
	hash, err := files.Hash("books/1/book.EPUB")

	require.NoError(t, err)
	assert.Len(t, hash, 64)
	assert.True(t, files.Exists("books/1/book.EPUB"))
	require.NoError(t, files.Remove("books/1"))
	assert.NoDirExists(t, filepath.Join(root, "books", "1"))
}

func TestFileService_Save_Rejects(t *testing.T) {
	root := t.TempDir()
	files := service.NewFileService(root)
	policy := domain.UploadPolicy{AllowedExts: []string{"epub"}, MaxSizeBytes: 3}

	tooLarge := files.Save(ctx, "a.epub", strings.NewReader("1234"), policy)
	badExt := files.Save(ctx, "a.pdf", strings.NewReader("1"), policy)

	assert.ErrorIs(t, tooLarge, apperrors.ErrFileTooLarge)
	assert.NoFileExists(t, filepath.Join(root, "a.epub"))
	assert.ErrorIs(t, badExt, apperrors.ErrUnsupportedFormat)
}

func TestFileService_Link_SharesContent(t *testing.T) {
	files := service.NewFileService(t.TempDir())
	policy := domain.UploadPolicy{AllowedExts: []string{"epub"}, MaxSizeBytes: 10}
	require.NoError(t, files.Save(ctx, "books/1/book.epub", strings.NewReader("data"), policy))

	require.NoError(t, files.Link("books/1/book.epub", "books/1/original.epub"))
	book, err := files.Hash("books/1/book.epub")
	require.NoError(t, err)
	original, err := files.Hash("books/1/original.epub")

	require.NoError(t, err)
	assert.Equal(t, book, original)
}
