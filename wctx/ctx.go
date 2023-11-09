// index for context values
package wctx

import "context"

type key int

const (
	chainIDKey  key = 1
	igNameKey   key = 2
	srcNameKey  key = 3
	backfillKey key = 4
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

func WithBackfill(ctx context.Context, b bool) context.Context {
	return context.WithValue(ctx, backfillKey, b)
}

func Backfill(ctx context.Context) bool {
	b, _ := ctx.Value(backfillKey).(bool)
	return b
}
