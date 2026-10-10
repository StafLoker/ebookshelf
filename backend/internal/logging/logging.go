package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

// TODO: dev/prod behaviour is driven only by [log] level in config.ini.
//   - Development: level = DEBUG prints pkg:Func:line and the accessLog entries (logged at DEBUG).
//   - Production: level = INFO (default) prints the line without pkg:Func:line, which is cheaper.
//
// Split into a separate setting if DEBUG logs are ever needed in prod without the caller info.

type correlationKey struct{}

type handler struct {
	w     io.Writer
	mu    *sync.Mutex
	level slog.Level
	attrs string
}

func New(w io.Writer, level string) (*slog.Logger, error) {
	levels := map[string]slog.Level{
		"DEBUG": slog.LevelDebug, "INFO": slog.LevelInfo, "WARNING": slog.LevelWarn, "ERROR": slog.LevelError,
	}

	l, ok := levels[strings.ToUpper(level)]
	if !ok {
		return nil, fmt.Errorf("logging: invalid level %q", level)
	}

	return slog.New(&handler{w: w, mu: &sync.Mutex{}, level: l}), nil
}

func WithCorrelation(ctx context.Context, kind string, id string) context.Context {
	return context.WithValue(ctx, correlationKey{}, kind+"="+id)
}

func NewID() (string, error) {
	var b [4]byte

	_, err := rand.Read(b[:])

	return hex.EncodeToString(b[:]), err
}

func (h *handler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level
}

func (h *handler) Handle(ctx context.Context, r slog.Record) error {
	var (
		b     strings.Builder
		corr  = "-"
		level = r.Level.String()
	)

	if c, ok := ctx.Value(correlationKey{}).(string); ok {
		corr = c
	}
	if r.Level == slog.LevelWarn {
		level = "WARNING"
	}

	fmt.Fprintf(&b, "%s | %-8s | %s | ", r.Time.UTC().Format("2006-01-02 15:04:05"), level, corr)
	if h.level <= slog.LevelDebug && r.PC != 0 {
		f, _ := runtime.CallersFrames([]uintptr{r.PC}).Next()
		name := f.Function[strings.LastIndex(f.Function, "/")+1:]
		pkg, _, _ := strings.Cut(name, ".")
		fmt.Fprintf(&b, "%s:%s:%d - ", pkg, name[strings.LastIndex(name, ".")+1:], f.Line)
	}
	b.WriteString(r.Message)
	b.WriteString(h.attrs)
	r.Attrs(func(a slog.Attr) bool {
		writeAttr(&b, a)
		return true
	})
	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())

	return err
}

func (h *handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var b strings.Builder

	b.WriteString(h.attrs)
	for _, a := range attrs {
		writeAttr(&b, a)
	}
	c := *h
	c.attrs = b.String()

	return &c
}

func (h *handler) WithGroup(string) slog.Handler {
	return h
}

func writeAttr(b *strings.Builder, a slog.Attr) {
	v := a.Value.Resolve().String()

	if v == "" || strings.ContainsAny(v, " |=\"") || strings.ContainsFunc(v, unicode.IsControl) {
		v = strconv.Quote(v)
	}
	b.WriteString(" | ")
	b.WriteString(a.Key)
	b.WriteString("=")
	b.WriteString(v)
}
