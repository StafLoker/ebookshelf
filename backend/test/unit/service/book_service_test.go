package service_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ebookshelf/internal/domain"
	"ebookshelf/internal/dto"
	apperrors "ebookshelf/internal/errors"
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
		actor:   dto.UserResponse{ID: actor.ID, Role: actor.Role},
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

	pending, err := svc.book.Upload(ctx, svc.actor.ID, "a.fb2", openFile(t, testutil.WriteFB2(t, t.TempDir(), title, "Ann")))
	require.NoError(t, err)
	_, err = svc.book.CommitPending(ctx, svc.actor, pending.ID)
	require.NoError(t, err)
}

func TestBookService_Upload_PendingUntilCommit(t *testing.T) {
	svc := newBookServices(t)
	path := testutil.WriteEPUB(t, t.TempDir(), "Title", "Ann")

	pending, err := svc.book.Upload(ctx, svc.actor.ID, "Title.epub", openFile(t, path))
	require.NoError(t, err)
	before, err := svc.book.List(ctx, svc.actor.ID, dto.BookFilterRequest{}, 1, 24)
	require.NoError(t, err)
	_, hidden := svc.book.Get(ctx, svc.actor.ID, pending.ID)
	added, err := svc.book.CommitPending(ctx, svc.actor, pending.ID)
	require.NoError(t, err)
	after, err := svc.book.List(ctx, svc.actor.ID, dto.BookFilterRequest{}, 1, 24)
	require.NoError(t, err)

	assert.Equal(t, domain.BookPending, pending.Status)
	assert.True(t, pending.HasCover)
	assert.FileExists(t, filepath.Join(svc.dataDir, "books", "1", "book.epub"))
	assert.FileExists(t, filepath.Join(svc.dataDir, "books", "1", "original.epub"))
	assert.FileExists(t, filepath.Join(svc.dataDir, "images", "covers", "1.png"))
	entries, _ := os.ReadDir(filepath.Join(svc.dataDir, "tmp"))
	assert.Empty(t, entries)
	assert.Equal(t, 0, before.Total)
	assert.ErrorIs(t, hidden, apperrors.ErrBookNotFound)
	assert.Equal(t, domain.BookAdded, added.Status)
	assert.Equal(t, 1, after.Total)
}

func TestBookService_Upload_DuplicateOfPending(t *testing.T) {
	svc := newBookServices(t)
	path := testutil.WriteFB2(t, t.TempDir(), "Title", "Ann")
	first, err := svc.book.Upload(ctx, svc.actor.ID, "a.fb2", openFile(t, path))
	require.NoError(t, err)

	_, err = svc.book.Upload(ctx, svc.actor.ID, "b.fb2", openFile(t, path))

	assert.ErrorIs(t, err, apperrors.ErrBookDuplicate)
	assert.Equal(t, map[string]int64{"book_id": first.ID}, err.(*apperrors.AppError).Details)
	entries, _ := os.ReadDir(filepath.Join(svc.dataDir, "tmp"))
	assert.Empty(t, entries)
}

func TestBookService_Upload_RejectsBadFiles(t *testing.T) {
	svc := newBookServices(t)

	_, unsupported := svc.book.Upload(ctx, svc.actor.ID, "book.pdf", strings.NewReader("x"))
	_, corrupt := svc.book.Upload(ctx, svc.actor.ID, "book.epub", strings.NewReader("not a zip"))

	assert.ErrorIs(t, unsupported, apperrors.ErrUnsupportedFormat)
	assert.ErrorIs(t, corrupt, apperrors.ErrUnreadableFile)
	assert.NoDirExists(t, filepath.Join(svc.dataDir, "books"))
}

func TestBookService_Get_OnlyUploaderAndAdmins(t *testing.T) {
	svc := newBookServices(t)
	pending, err := svc.book.Upload(ctx, svc.actor.ID, "a.fb2", openFile(t, testutil.WriteFB2(t, t.TempDir(), "Title", "Ann")))
	require.NoError(t, err)

	_, otherErr := svc.book.GetPending(ctx, svc.other, pending.ID)
	_, adminErr := svc.book.GetPending(ctx, dto.UserResponse{ID: 99, Role: domain.RoleAdmin}, pending.ID)
	mine, err := svc.book.ListPending(ctx, svc.actor)
	require.NoError(t, err)
	others, err := svc.book.ListPending(ctx, svc.other)
	require.NoError(t, err)

	assert.ErrorIs(t, otherErr, apperrors.ErrBookNotFound)
	assert.NoError(t, adminErr)
	assert.Len(t, mine, 1)
	assert.Empty(t, others)
}

func TestBookService_Discard_RemovesRowAndFiles(t *testing.T) {
	svc := newBookServices(t)
	pending, err := svc.book.Upload(ctx, svc.actor.ID, "Title.epub", openFile(t, testutil.WriteEPUB(t, t.TempDir(), "Title", "Ann")))
	require.NoError(t, err)

	require.NoError(t, svc.book.DiscardPending(ctx, svc.actor, pending.ID))
	_, err = svc.book.GetPending(ctx, svc.actor, pending.ID)

	assert.ErrorIs(t, err, apperrors.ErrBookNotFound)
	assert.NoDirExists(t, filepath.Join(svc.dataDir, "books", "1"))
	assert.NoFileExists(t, filepath.Join(svc.dataDir, "images", "covers", "1.png"))
}

func TestBookService_MarkRead_ListedAsRead(t *testing.T) {
	svc := newBookServices(t)
	commitFB2(t, svc, "Title")

	require.NoError(t, svc.book.MarkRead(ctx, svc.actor.ID, 1))
	read, err := svc.book.List(ctx, svc.actor.ID, dto.BookFilterRequest{Read: "read"}, 1, 24)

	require.NoError(t, err)
	assert.Equal(t, 1, read.Total)
	assert.ErrorIs(t, svc.book.MarkRead(ctx, svc.actor.ID, 99), apperrors.ErrBookNotFound)
}

func TestBookService_File_SanitizedName(t *testing.T) {
	svc := newBookServices(t)
	commitFB2(t, svc, "Ti/tle")

	file, err := svc.book.File(ctx, 1)

	require.NoError(t, err)
	assert.Equal(t, "Ti_tle - Ann.fb2", file.Name)
}
