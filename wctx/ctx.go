// index for context values
package wctx

import "context"

type key int

const (
	taskIDKey  key = 1
	chainIDKey key = 2
)

func WithChainID(ctx context.Context, id uint64) context.Context {
	return context.WithValue(ctx, chainIDKey, id)
}

func ChainID(ctx context.Context) uint64 {
	id, _ := ctx.Value(chainIDKey).(uint64)
	return id
}
func WithTaskID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, taskIDKey, id)
}

func TaskID(ctx context.Context) string {
	id, _ := ctx.Value(taskIDKey).(string)
	return id
}
