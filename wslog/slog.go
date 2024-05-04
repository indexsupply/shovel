// custom slog handler
//
// Adapted from: https://github.com/jba/slog
// BSD 3-Clause License
// Copyright (c) 2022, Jonathan Amsterdam
package wslog

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

type Handler struct {
	ctxs      []func(context.Context) (string, any)
	opts      slog.HandlerOptions
	prefix    string
	preformat string
	mu        sync.Mutex
	w         io.Writer
}

func New(w io.Writer, opts *slog.HandlerOptions) *Handler {
	h := &Handler{w: w}
	if opts != nil {
		h.opts = *opts
	}
	if h.opts.ReplaceAttr == nil {
		h.opts.ReplaceAttr = func(_ []string, a slog.Attr) slog.Attr { return a }
	}
	return h
}

// Registers a context value with the handler.
// The ckey argument is used to retreive a value from the context
// and name is used as the key in the log line.
// For example: ContextKey(chainIDKey, "chain") becomes: chain=XXX
func (h *Handler) RegisterContext(f func(context.Context) (string, any)) {
	h.mu.Lock()
	h.ctxs = append(h.ctxs, f)
	h.mu.Unlock()
}

func (h *Handler) Enabled(ctx context.Context, level slog.Level) bool {
	minLevel := slog.LevelInfo
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return level >= minLevel
}

func (h *Handler) WithGroup(name string) slog.Handler {
	var ctxs []func(context.Context) (string, any)
	for i := range h.ctxs {
		ctxs = append(ctxs, h.ctxs[i])
	}
	return &Handler{
		ctxs:      ctxs,
		opts:      h.opts,
		prefix:    h.prefix + name + ".",
		preformat: h.preformat,
		mu:        sync.Mutex{},
		w:         h.w,
	}
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	var buf []byte
	for _, a := range attrs {
		buf = h.appendAttr(buf, h.prefix, a)
	}
	var ctxs []func(context.Context) (string, any)
	for i := range h.ctxs {
		ctxs = append(ctxs, h.ctxs[i])
	}
	return &Handler{
		ctxs:      ctxs,
		opts:      h.opts,
		prefix:    h.prefix,
		preformat: h.preformat + string(buf),
		mu:        sync.Mutex{},
		w:         h.w,
	}
}

var bpool = sync.Pool{New: func() any { b := make([]byte, 0, 1024); return &b }}

func freebuf(b *[]byte) {
	if cap(*b) <= 16<<10 {
		*b = (*b)[:0]
		bpool.Put(b)
	}
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	var (
		bufp = bpool.Get().(*[]byte)
		buf  = *bufp
	)
	defer func() {
		*bufp = buf
		freebuf(bufp)
	}()
	buf = append(buf, []byte("l=")...)
	buf = append(buf, []byte(fmt.Sprintf("%-5s", strings.ToLower(r.Level.String())))...)
	buf = append(buf, ' ')
	buf = append(buf, h.preformat...)

	if len(r.Message) > 0 {
		buf = append(buf, 'm', 's', 'g', '=')
		buf = append(buf, []byte(r.Message)...)
		buf = append(buf, ' ')
	}
	for _, f := range h.ctxs {
		k, v := f(ctx)
		if k == "" {
			continue
		}
		buf = fmt.Appendf(buf, "%s=%v ", k, v)
	}
	if h.opts.AddSource && r.PC != 0 {
		fs := runtime.CallersFrames([]uintptr{r.PC})
		f, _ := fs.Next()
		buf = append(buf, f.File...)
		buf = append(buf, ':')
		buf = strconv.AppendInt(buf, int64(f.Line), 10)
		buf = append(buf, ' ')
	}
	r.Attrs(func(a slog.Attr) bool {
		buf = h.appendAttr(buf, h.prefix, a)
		return true
	})
	buf = bytes.TrimSuffix(buf, []byte(" "))
	buf = append(buf, '\n')
	h.mu.Lock()
	_, err := h.w.Write(buf)
	h.mu.Unlock()
	return err
}

func (h *Handler) appendAttr(buf []byte, prefix string, a slog.Attr) []byte {
	if a.Equal(slog.Attr{}) {
		return buf
	}
	if a.Value.Kind() != slog.KindGroup {
		buf = append(buf, prefix...)
		buf = append(buf, a.Key...)
		buf = append(buf, '=')
		return fmt.Appendf(buf, "%v ", a.Value.Any())
	}
	// Group
	if a.Key != "" {
		prefix += a.Key + "."
	}
	for _, a := range a.Value.Group() {
		buf = h.appendAttr(buf, prefix, a)
	}
	return buf
}
