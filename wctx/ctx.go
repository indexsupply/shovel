// index for context values
package wctx

import "context"

type key int

const (
	chainIDKey  key = 1
	intgNameKey key = 2
	srcNameKey  key = 3
)

func WithChainID(ctx context.Context, id uint64) context.Context {
	return context.WithValue(ctx, chainIDKey, id)
}

func ChainID(ctx context.Context) uint64 {
	id, _ := ctx.Value(chainIDKey).(uint64)
	return id
}

func WithIntgName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, intgNameKey, name)
}

func IntgName(ctx context.Context) string {
	name, _ := ctx.Value(intgNameKey).(string)
	return name
}

func WithSrcName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, srcNameKey, name)
}

func SrcName(ctx context.Context) string {
	name, _ := ctx.Value(srcNameKey).(string)
	return name
}
