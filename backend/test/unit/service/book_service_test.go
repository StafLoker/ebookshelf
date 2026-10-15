package service_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ebookshelf/internal/domain"
	"ebookshelf/internal/dto"
	apperrors "ebookshelf/internal/errors"
	"ebookshelf/internal/formats"
	"ebookshelf/internal/repository"
	"ebookshelf/internal/service"
	"ebookshelf/test/testutil"
)

type bookServices struct {
	book    *service.BookService
	dataDir string
	actor   dto.UserResponse
	other   dto.UserResponse
}

func newBookServices(t *testing.T) bookServices {
	t.Helper()

	db := testutil.OpenTempDB(t)
	bookRepo := repository.NewBookRepository(db)
	userRepo := repository.NewUserRepository(db)
	dataDir := t.TempDir()
	files := service.NewFileService(dataDir)
	actor := seedUser(t, userRepo, "reader", domain.RoleUser)
	other := seedUser(t, userRepo, "other", domain.RoleUser)
	return bookServices{
		book:    service.NewBookService(bookRepo, repository.NewReadStatusRepository(db), files),
		dataDir: dataDir,
		actor:   dto.UserResponse{ID: actor.ID, Role: actor.Role, CanAdd: true, CanEdit: true, CanDelete: true},
		other:   dto.UserResponse{ID: other.ID, Role: other.Role},
	}
}

func openFile(t *testing.T, path string) *os.File {
	t.Helper()

	f, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func commitFB2(t *testing.T, svc bookServices, title string) {
	t.Helper()

	pending, err := svc.book.Add(ctx, svc.actor.ID, "a.fb2", openFile(t, testutil.WriteFB2(t, t.TempDir(), title, "Ann")))
	require.NoError(t, err)
	_, err = svc.book.Commit(ctx, svc.actor, pending.ID)
	require.NoError(t, err)
}

// the metadata of the books from commitFB2 with a new title
func editRequest(title string) dto.UpdateBookRequest {
	return dto.UpdateBookRequest{Title: title, Language: "ru", Authors: []dto.Author{{LastName: "Ann"}}}
}

func TestBookService_Add_PendingUntilCommit(t *testing.T) {
	svc := newBookServices(t)
	path := testutil.WriteEPUB(t, t.TempDir(), "Title", "Ann")

	pending, err := svc.book.Add(ctx, svc.actor.ID, "Title.epub", openFile(t, path))
	require.NoError(t, err)
	before, err := svc.book.List(ctx, svc.actor, dto.BookFilterRequest{}, 1, 24)
	require.NoError(t, err)
	added, err := svc.book.Commit(ctx, svc.actor, pending.ID)
	require.NoError(t, err)
	after, err := svc.book.List(ctx, svc.actor, dto.BookFilterRequest{}, 1, 24)
	require.NoError(t, err)

	assert.Equal(t, domain.BookPending, pending.Status)
	assert.True(t, pending.HasCover)
	assert.FileExists(t, filepath.Join(svc.dataDir, "books", "1", "book.epub"))
	assert.FileExists(t, filepath.Join(svc.dataDir, "books", "1", "original.epub"))
	assert.FileExists(t, filepath.Join(svc.dataDir, "images", "covers", "1.png"))
	entries, _ := os.ReadDir(filepath.Join(svc.dataDir, "tmp"))
	assert.Empty(t, entries)
	assert.Equal(t, 0, before.Total)
	assert.Equal(t, domain.BookAdded, added.Status)
	assert.Equal(t, 1, after.Total)
}

func TestBookService_Add_DuplicateOfPending(t *testing.T) {
	svc := newBookServices(t)
	path := testutil.WriteFB2(t, t.TempDir(), "Title", "Ann")
	first, err := svc.book.Add(ctx, svc.actor.ID, "a.fb2", openFile(t, path))
	require.NoError(t, err)

	_, err = svc.book.Add(ctx, svc.actor.ID, "b.fb2", openFile(t, path))

	assert.ErrorIs(t, err, apperrors.ErrBookDuplicate)
	assert.Equal(t, map[string]int64{"book_id": first.ID}, err.(*apperrors.AppError).Details)
	entries, _ := os.ReadDir(filepath.Join(svc.dataDir, "tmp"))
	assert.Empty(t, entries)
}

func TestBookService_Add_DuplicateOfEditedBook(t *testing.T) {
	svc := newBookServices(t)
	path := testutil.WriteFB2(t, t.TempDir(), "Title", "Ann")
	pending, err := svc.book.Add(ctx, svc.actor.ID, "a.fb2", openFile(t, path))
	require.NoError(t, err)
	_, err = svc.book.Update(ctx, svc.actor, pending.ID, editRequest("New Title"))
	require.NoError(t, err)

	_, err = svc.book.Add(ctx, svc.actor.ID, "a.fb2", openFile(t, path))

	assert.ErrorIs(t, err, apperrors.ErrBookDuplicate)
}

func TestBookService_Add_RejectsBadFiles(t *testing.T) {
	svc := newBookServices(t)

	_, unsupported := svc.book.Add(ctx, svc.actor.ID, "book.pdf", strings.NewReader("x"))
	_, corrupt := svc.book.Add(ctx, svc.actor.ID, "book.epub", strings.NewReader("not a zip"))

	assert.ErrorIs(t, unsupported, apperrors.ErrUnsupportedFormat)
	assert.ErrorIs(t, corrupt, apperrors.ErrUnreadableFile)
	assert.NoDirExists(t, filepath.Join(svc.dataDir, "books"))
}

func TestBookService_Get_OnlyUploaderAndAdmins(t *testing.T) {
	svc := newBookServices(t)
	pending, err := svc.book.Add(ctx, svc.actor.ID, "a.fb2", openFile(t, testutil.WriteFB2(t, t.TempDir(), "Title", "Ann")))
	require.NoError(t, err)

	_, otherErr := svc.book.Get(ctx, svc.other, pending.ID)
	_, adminErr := svc.book.Get(ctx, dto.UserResponse{ID: 99, Role: domain.RoleAdmin}, pending.ID)
	mine, err := svc.book.List(ctx, svc.actor, dto.BookFilterRequest{Status: "pending"}, 1, 24)
	require.NoError(t, err)
	others, err := svc.book.List(ctx, svc.other, dto.BookFilterRequest{Status: "pending"}, 1, 24)
	require.NoError(t, err)

	assert.ErrorIs(t, otherErr, apperrors.ErrBookNotFound)
	assert.NoError(t, adminErr)
	assert.Equal(t, 1, mine.Total)
	assert.Equal(t, 0, others.Total)
}

func TestBookService_File_SanitizedName(t *testing.T) {
	svc := newBookServices(t)
	commitFB2(t, svc, "Ti/tle")

	file, err := svc.book.File(ctx, svc.actor, 1)

	require.NoError(t, err)
	assert.Equal(t, "Ti_tle - Ann.fb2", file.Name)
}

func TestBookService_Update_WritesFileThenDatabase(t *testing.T) {
	svc := newBookServices(t)
	commitFB2(t, svc, "Title")
	original, err := os.ReadFile(filepath.Join(svc.dataDir, "books", "1", "original.fb2"))
	require.NoError(t, err)
	before, err := svc.book.File(ctx, svc.actor, 1)
	require.NoError(t, err)
	book, err := svc.book.Update(ctx, svc.actor, 1, dto.UpdateBookRequest{Title: "New Title", Language: "ru", Authors: []dto.Author{
		{FirstName: "Lev", MiddleName: "Nikolaevich", LastName: "Tolstoy"},
		{FirstName: "LEV", MiddleName: "nikolaevich", LastName: "TOLSTOY"},
	}})
	require.NoError(t, err)
	after, err := svc.book.File(ctx, svc.actor, 1)
	require.NoError(t, err)
	meta, err := formats.For("fb2").Read(after.Path)
	require.NoError(t, err)
	kept, err := os.ReadFile(filepath.Join(svc.dataDir, "books", "1", "original.fb2"))
	require.NoError(t, err)

	assert.Equal(t, "New Title", book.Title)
	assert.Equal(t, "Lev Nikolaevich Tolstoy", book.Authors[0].Name)
	assert.Len(t, book.Authors, 1)
	assert.Equal(t, "New Title", meta.Title)
	assert.Equal(t, []domain.Author{{FirstName: "Lev", MiddleName: "Nikolaevich", LastName: "Tolstoy"}}, meta.Authors)
	assert.NotEqual(t, before.Hash, after.Hash)
	assert.Equal(t, original, kept)
}

func TestBookService_Update_LibraryBookNeedsEditPermission(t *testing.T) {
	svc := newBookServices(t)
	commitFB2(t, svc, "Title")
	reader := dto.UserResponse{ID: svc.actor.ID, Role: domain.RoleUser, CanAdd: true}

	_, updateErr := svc.book.Update(ctx, reader, 1, editRequest("New Title"))
	deleteErr := svc.book.Delete(ctx, reader, 1)

	assert.ErrorIs(t, updateErr, apperrors.ErrForbidden)
	assert.ErrorIs(t, deleteErr, apperrors.ErrForbidden)
}

func TestBookService_Update_WriteFailureLeavesBook(t *testing.T) {
	svc := newBookServices(t)
	commitFB2(t, svc, "Title")
	tmp := filepath.Join(svc.dataDir, "tmp")
	require.NoError(t, os.Chmod(tmp, 0o500))
	t.Cleanup(func() { _ = os.Chmod(tmp, 0o755) })
	_, err := svc.book.Update(ctx, svc.actor, 1, editRequest("New Title"))
	book, getErr := svc.book.Get(ctx, svc.actor, 1)

	assert.ErrorIs(t, err, apperrors.ErrFileWriteFailed)
	require.NoError(t, getErr)
	assert.Equal(t, "Title", book.Title)
}

func TestBookService_Update_MoveFailureRollsBack(t *testing.T) {
	svc := newBookServices(t)
	commitFB2(t, svc, "Title")
	dir := filepath.Join(svc.dataDir, "books", "1")
	before, err := os.ReadFile(filepath.Join(dir, "book.fb2"))
	require.NoError(t, err)
	require.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	_, err = svc.book.Update(ctx, svc.actor, 1, editRequest("New Title"))
	book, getErr := svc.book.Get(ctx, svc.actor, 1)
	after, readErr := os.ReadFile(filepath.Join(dir, "book.fb2"))

	assert.ErrorIs(t, err, apperrors.ErrInternal)
	require.NoError(t, getErr)
	require.NoError(t, readErr)
	assert.Equal(t, "Title", book.Title)
	assert.Equal(t, before, after)
}

func TestBookService_SetCover_ReplacesPendingCover(t *testing.T) {
	svc := newBookServices(t)
	pending, err := svc.book.Add(ctx, svc.actor.ID, "Title.epub", openFile(t, testutil.WriteEPUB(t, t.TempDir(), "Title", "Ann")))
	require.NoError(t, err)
	cover := testutil.PNG(t, 5)

	_, err = svc.book.SetCover(ctx, svc.actor, pending.ID, bytes.NewReader(cover))
	require.NoError(t, err)
	_, notImage := svc.book.SetCover(ctx, svc.actor, pending.ID, strings.NewReader("text"))
	stored, err := os.ReadFile(filepath.Join(svc.dataDir, "images", "covers", "1.png"))
	require.NoError(t, err)
	meta, err := formats.For("epub").Read(filepath.Join(svc.dataDir, "books", "1", "book.epub"))
	require.NoError(t, err)

	assert.Equal(t, cover, stored)
	assert.Equal(t, cover, meta.Cover)
	assert.ErrorIs(t, notImage, apperrors.ErrUnsupportedFormat)
}

func TestBookService_Delete_DiscardsPendingRowAndFiles(t *testing.T) {
	svc := newBookServices(t)
	pending, err := svc.book.Add(ctx, svc.actor.ID, "Title.epub", openFile(t, testutil.WriteEPUB(t, t.TempDir(), "Title", "Ann")))
	require.NoError(t, err)

	require.NoError(t, svc.book.Delete(ctx, svc.actor, pending.ID))
	_, err = svc.book.Get(ctx, svc.actor, pending.ID)

	assert.ErrorIs(t, err, apperrors.ErrBookNotFound)
	assert.NoDirExists(t, filepath.Join(svc.dataDir, "books", "1"))
	assert.NoFileExists(t, filepath.Join(svc.dataDir, "images", "covers", "1.png"))
}

func TestBookService_MarkRead_ListedAsRead(t *testing.T) {
	svc := newBookServices(t)
	commitFB2(t, svc, "Title")

	pending, err := svc.book.Add(ctx, svc.actor.ID, "b.fb2", openFile(t, testutil.WriteFB2(t, t.TempDir(), "Other", "Ann")))
	require.NoError(t, err)

	require.NoError(t, svc.book.MarkRead(ctx, svc.actor, 1))
	read, err := svc.book.List(ctx, svc.actor, dto.BookFilterRequest{Read: "read"}, 1, 24)

	require.NoError(t, err)
	assert.Equal(t, 1, read.Total)
	assert.ErrorIs(t, svc.book.MarkRead(ctx, svc.actor, 99), apperrors.ErrBookNotFound)
	assert.ErrorIs(t, svc.book.MarkRead(ctx, svc.actor, pending.ID), apperrors.ErrBookNotFound)
}
