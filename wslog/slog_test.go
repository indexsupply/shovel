package wslog

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"kr.dev/diff"
)

type wslogTestKey struct{}

func TestHandler_Context(t *testing.T) {
	ctx := context.Background()
	buf := bytes.Buffer{}
	clg := New(&buf, nil)
	clg.RegisterContext(func(ctx context.Context) (string, any) {
		return "foo", ctx.Value(wslogTestKey{})
	})
	log := slog.New(clg)

	ctx = context.WithValue(ctx, wslogTestKey{}, "bar")
	log.InfoContext(ctx, "")
	diff.Test(t, t.Errorf, buf.String(), "foo=bar\n")
}

func TestHandler(t *testing.T) {
	cases := []struct {
		name  string
		with  func(*slog.Logger) *slog.Logger
		msg   string
		attrs []slog.Attr
		want  string
	}{
		{
			name:  "basic",
			attrs: []slog.Attr{slog.String("foo", "bar")},
			msg:   "baz",
			want:  "msg=baz foo=bar\n",
		},
		{
			name: "group",
			attrs: []slog.Attr{
				slog.String("foo", "bar"),
				slog.Group("baz", slog.Int("a", 1), slog.Int("b", 2)),
				slog.Bool("qux", true),
			},
			want: "foo=bar baz.a=1 baz.b=2 qux=true\n",
		},
		{
			name:  "WithAttrs",
			with:  func(l *slog.Logger) *slog.Logger { return l.With("wa", 1, "wb", 2) },
			attrs: []slog.Attr{slog.String("c", "foo"), slog.Bool("b", true)},
			want:  "wa=1 wb=2 c=foo b=true\n",
		},
		{
			name: "WithAttrs,WithGroup",
			with: func(l *slog.Logger) *slog.Logger {
				return l.With("wa", 1, "wb", 2).WithGroup("p1").With("wc", 3).WithGroup("p2")
			},
			attrs: []slog.Attr{slog.String("c", "foo"), slog.Bool("b", true)},
			want:  "wa=1 wb=2 p1.wc=3 p1.p2.c=foo p1.p2.b=true\n",
		},
	}

	for _, tc := range cases {
		var (
			ctx = context.Background()
			buf bytes.Buffer
			l   = slog.New(New(&buf, nil))
		)
		if tc.with != nil {
			l = tc.with(l)
		}
		l.LogAttrs(ctx, slog.LevelInfo, tc.msg, tc.attrs...)
		diff.Test(t, t.Errorf, buf.String(), tc.want)
	}
}
