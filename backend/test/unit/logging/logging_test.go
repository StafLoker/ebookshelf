package logging_test

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ebookshelf/internal/logging"
)

func TestLogger_Info_ExactLine(t *testing.T) {
	var buf bytes.Buffer
	log, err := logging.New(&buf, "info")
	require.NoError(t, err)
	ctx := logging.WithCorrelation(context.Background(), "req", "ab12cd34")
	r := slog.NewRecord(time.Date(2026, 9, 22, 23, 7, 34, 0, time.UTC), slog.LevelInfo, "Book added", 0)
	r.AddAttrs(slog.Int("book_id", 12), slog.String("title", "Dune Messiah"))

	err = log.Handler().Handle(ctx, r)

	require.NoError(t, err)
	assert.Equal(t, "2026-09-22 23:07:34 | INFO     | req=ab12cd34 | Book added | book_id=12 | title=\"Dune Messiah\"\n", buf.String())
}

func TestLogger_Info_NewlineInValueIsQuoted(t *testing.T) {
	var buf bytes.Buffer
	log, err := logging.New(&buf, "INFO")
	require.NoError(t, err)
	r := slog.NewRecord(time.Date(2026, 9, 22, 23, 7, 34, 0, time.UTC), slog.LevelInfo, "request", 0)
	r.AddAttrs(slog.String("path", "/api/x\n2026-01-01 | ERROR | forged"))

	err = log.Handler().Handle(context.Background(), r)

	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(buf.String(), "\n"))
}
