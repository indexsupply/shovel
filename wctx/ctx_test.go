package wctx

import (
	"context"
	"testing"

	"github.com/indexsupply/shovel/tc"
)

func TestCounter(t *testing.T) {
	ctr := uint64(0)
	ctx := WithCounter(context.Background(), &ctr)
	CounterAdd(ctx, 1)
	func(inner context.Context) { CounterAdd(inner, 1) }(ctx)
	CounterAdd(ctx, 1)
	tc.WantGot(t, uint64(3), Counter(ctx))
}
