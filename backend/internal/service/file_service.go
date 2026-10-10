package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"ebookshelf/internal/domain"
	apperrors "ebookshelf/internal/errors"
)

const (
	dirMode  = 0o755
	fileMode = 0o644

	tmpDir    = "tmp"
	booksDir  = "books"
	coversDir = "images/covers"
)

// Paths are relative to root and built by the layout methods, never taken from the client.
type FileService struct {
	root string
}

func NewFileService(root string) *FileService {
	return &FileService{root: root}
}

func (s *FileService) Save(ctx context.Context, relPath string, src io.Reader, policy domain.UploadPolicy) error {
	var err error

	if ext := fileExt(relPath); !slices.Contains(policy.AllowedExts, ext) {
		err = apperrors.UnsupportedFormat(fmt.Sprintf("format %q is not supported", ext))
	} else {
		err = s.write(ctx, s.Path(relPath), src, policy.MaxSizeBytes)
	}
	return err
}

func (s *FileService) Link(from, to string) error {
	return os.Link(s.Path(from), s.Path(to))
}

func (s *FileService) Move(from, to string) error {
	dst := s.Path(to)
	err := os.MkdirAll(filepath.Dir(dst), dirMode)
	if err == nil {
		err = os.Rename(s.Path(from), dst)
	}
	return err
}

// a missing path is not an error, so cleanup can run unconditionally
func (s *FileService) Remove(relPath string) error {
	return os.RemoveAll(s.Path(relPath))
}

func (s *FileService) Exists(relPath string) bool {
	_, err := os.Stat(s.Path(relPath))
	return err == nil
}

func (s *FileService) Hash(relPath string) (string, error) {
	var sum string

	f, err := os.Open(s.Path(relPath))
	if err == nil {
		defer func() { _ = f.Close() }()
		h := sha256.New()
		if _, err = io.Copy(h, f); err == nil {
			sum = hex.EncodeToString(h.Sum(nil))
		}
	}
	return sum, err
}

// a fresh path in tmp/ for a file still being received
func (*FileService) NewTmpPath(ext string) string {
	return path.Join(tmpDir, "upload-"+rand.Text()+"."+ext)
}

// tmp/ only holds files cut off mid-upload, so it is emptied at startup
func (s *FileService) ClearTmp(ctx context.Context) {
	if err := s.Remove(tmpDir); err != nil {
		slog.WarnContext(ctx, "clear tmp failed", "err", err)
	}
}

func (s *FileService) Path(relPath string) string {
	return filepath.Join(s.root, relPath)
}

func (*FileService) BookDir(id int64) string {
	return path.Join(booksDir, strconv.FormatInt(id, 10))
}

func (s *FileService) BookPath(id int64, ext string) string {
	return path.Join(s.BookDir(id), "book."+ext)
}

func (s *FileService) OriginalPath(id int64, ext string) string {
	return path.Join(s.BookDir(id), "original."+ext)
}

func (*FileService) CoverPath(id int64, ext string) string {
	return path.Join(coversDir, strconv.FormatInt(id, 10)+"."+ext)
}

func (s *FileService) write(ctx context.Context, abs string, src io.Reader, maxBytes int64) error {
	var (
		out *os.File
		n   int64
		err error
	)

	if err = os.MkdirAll(filepath.Dir(abs), dirMode); err == nil {
		out, err = os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode)
	}
	if err == nil {
		n, err = io.Copy(out, io.LimitReader(src, maxBytes+1))
		if closeErr := out.Close(); err == nil {
			err = closeErr
		}
	}
	switch {
	case err != nil:
		slog.ErrorContext(ctx, "save file failed", "err", err)
		err = apperrors.Internal("failed to save file")
	case n > maxBytes:
		err = apperrors.FileTooLarge(fmt.Sprintf("file must not exceed %d MB", maxBytes/domain.MB))
	}
	if err != nil && out != nil {
		if rmErr := os.Remove(abs); rmErr != nil {
			slog.WarnContext(ctx, "remove partial file failed", "err", rmErr)
		}
	}
	return err
}

func fileExt(name string) string {
	return strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
}
