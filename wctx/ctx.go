// index for context values
package wctx

import (
	"context"
	"time"
)

type key int

const (
	chainIDKey  key = 1
	igNameKey   key = 2
	srcNameKey  key = 3
	versionKey  key = 4
	starTimeKey key = 5
)

func WithChainID(ctx context.Context, id uint64) context.Context {
	return context.WithValue(ctx, chainIDKey, id)
}

func ChainID(ctx context.Context) uint64 {
	id, _ := ctx.Value(chainIDKey).(uint64)
	return id
}

func WithIGName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, igNameKey, name)
}

func IGName(ctx context.Context) string {
	name, _ := ctx.Value(igNameKey).(string)
	return name
}

func WithSrcName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, srcNameKey, name)
}

func SrcName(ctx context.Context) string {
	name, _ := ctx.Value(srcNameKey).(string)
	return name
}

func WithVersion(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, versionKey, v)
}

func Version(ctx context.Context) string {
	v, _ := ctx.Value(versionKey).(string)
	return v
}

func WithStartTime(ctx context.Context, t time.Time) context.Context {
	return context.WithValue(ctx, starTimeKey, t)
}

func StartTime(ctx context.Context) time.Time {
	t, _ := ctx.Value(starTimeKey).(time.Time)
	return t
}
