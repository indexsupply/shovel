// index for context values
package wctx

import (
	"context"
	"sync/atomic"
)

type key int

const (
	chainIDKey  key = 1
	igNameKey   key = 2
	srcNameKey  key = 3
	versionKey  key = 4
	counterKey  key = 5
	numLimitKey key = 6
	srcURLKey   key = 7
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

func WithCounter(ctx context.Context, c *uint64) context.Context {
	return context.WithValue(ctx, counterKey, c)
}

func CounterAdd(ctx context.Context, n uint64) uint64 {
	cptr, ok := ctx.Value(counterKey).(*uint64)
	if !ok {
		return 0
	}
	return atomic.AddUint64(cptr, n)
}

func Counter(ctx context.Context) uint64 {
	cptr, ok := ctx.Value(counterKey).(*uint64)
	if !ok {
		return 0
	}
	return *cptr
}

type numLimit struct{ num, limit uint64 }

func WithNumLimit(ctx context.Context, n, l uint64) context.Context {
	return context.WithValue(ctx, numLimitKey, numLimit{n, l})
}

func NumLimit(ctx context.Context) (uint64, uint64) {
	nl, _ := ctx.Value(numLimitKey).(numLimit)
	return nl.num, nl.limit
}

func WithSrcURL(ctx context.Context, v string) context.Context {
	return context.WithValue(ctx, srcURLKey, v)
}

func SrcURL(ctx context.Context) string {
	v, _ := ctx.Value(srcURLKey).(string)
	return v
}
