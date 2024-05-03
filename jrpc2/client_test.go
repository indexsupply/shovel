package jrpc2

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/indexsupply/x/eth"
	"github.com/indexsupply/x/shovel/glf"
	"github.com/indexsupply/x/tc"
	"golang.org/x/sync/errgroup"
	"kr.dev/diff"
)

func init() {
	h := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(h))
}

type testGetter struct {
	callCount int
}

func (tg *testGetter) get(ctx context.Context, start, limit uint64) ([]eth.Block, error) {
	tg.callCount++

	var res []eth.Block
	for i := uint64(0); i < limit; i++ {
		res = append(res, eth.Block{Header: eth.Header{Number: eth.Uint64(start + i)}})
	}
	return res, nil
}

func TestCache_Prune(t *testing.T) {
	ctx := context.Background()
	tg := testGetter{}
	c := cache{maxreads: 2}
	blocks, err := c.get(false, ctx, 1, 1, tg.get)
	diff.Test(t, t.Fatalf, nil, err)
	diff.Test(t, t.Errorf, 1, len(blocks))
	diff.Test(t, t.Errorf, 1, tg.callCount)
	diff.Test(t, t.Errorf, 1, len(c.segments))

	for i := uint64(0); i < 9; i++ {
		blocks, err := c.get(false, ctx, 2+i, 1, tg.get)
		diff.Test(t, t.Fatalf, nil, err)
		diff.Test(t, t.Errorf, 1, len(blocks))
	}
	diff.Test(t, t.Errorf, 5, len(c.segments))

	var got []eth.Block
	for _, seg := range c.segments {
		for _, b := range seg.d {
			got = append(got, b)
		}
	}
	sort.Slice(got, func(i, j int) bool {
		return got[i].Num() < got[j].Num()
	})
	diff.Test(t, t.Errorf, got, []eth.Block{
		eth.Block{Header: eth.Header{Number: 6}},
		eth.Block{Header: eth.Header{Number: 7}},
		eth.Block{Header: eth.Header{Number: 8}},
		eth.Block{Header: eth.Header{Number: 9}},
		eth.Block{Header: eth.Header{Number: 10}},
	})
}

func TestCache_MaxReads(t *testing.T) {
	var (
		ctx = context.Background()
		tg  = testGetter{}
		c   = cache{maxreads: 2}
	)
	_, err := c.get(false, ctx, 1, 1, tg.get)
	tc.NoErr(t, err)
	tc.WantGot(t, 1, tg.callCount)

	_, err = c.get(false, ctx, 1, 1, tg.get)
	tc.NoErr(t, err)
	tc.WantGot(t, 1, tg.callCount)

	_, err = c.get(false, ctx, 1, 1, tg.get)
	tc.NoErr(t, err)
	tc.WantGot(t, 2, tg.callCount)
}

var (
	//go:embed testdata/block-18000000.json
	block18000000JSON string
	//go:embed testdata/logs-18000000.json
	logs18000000JSON string

	//go:embed testdata/block-1000001.json
	block1000001JSON string
	//go:embed testdata/logs-1000001.json
	logs1000001JSON string
)

func methodsMatch(t *testing.T, body []byte, want ...string) bool {
	var req []request

	if err := json.Unmarshal(body, &req); err != nil {
		var r request
		if err := json.Unmarshal(body, &r); err != nil {
			t.Fatal("unable to decode json into a request or []request")
		}
		req = append(req, r)
	}

	var methods []string
	for i := range req {
		methods = append(methods, req[i].Method)
	}
	t.Logf("methods=%#v", methods)
	return slices.Equal(methods, want)
}

func TestLatest_Cached(t *testing.T) {
	var counter int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		diff.Test(t, t.Fatalf, nil, err)
		switch {
		case methodsMatch(t, body, "eth_getBlockByNumber"):
			switch counter {
			case 0:
				_, err := w.Write([]byte(`{"result": {
					"hash": "0x95b198e154acbfc64109dfd22d8224fe927fd8dfdedfae01587674482ba4baf3",
					"number": "0x112a880"
				}}`))
				diff.Test(t, t.Fatalf, nil, err)
			case 1:
				_, err := w.Write([]byte(`{"result": {
					"hash": "0xd5ca78be6c6b42cf929074f502cef676372c26f8d0ba389b6f9b5d612d70f815",
					"number": "0x112a881"
				}}`))
				diff.Test(t, t.Fatalf, nil, err)
			}
		}
		counter++
	}))
	defer ts.Close()
	ctx := context.Background()
	c := New(ts.URL).WithMaxReads(1)

	n, h, err := c.Latest(ctx, 0)
	diff.Test(t, t.Errorf, nil, err)
	diff.Test(t, t.Errorf, counter, 1)
	diff.Test(t, t.Errorf, n, uint64(18000000))
	diff.Test(t, t.Errorf, eth.EncodeHex(h), "0x95b198e154acbfc64109dfd22d8224fe927fd8dfdedfae01587674482ba4baf3")

	n, _, err = c.Latest(ctx, 18000000-1)
	diff.Test(t, t.Errorf, nil, err)
	diff.Test(t, t.Errorf, counter, 1)
	diff.Test(t, t.Errorf, n, uint64(18000000))

	n, h, err = c.Latest(ctx, 18000000)
	diff.Test(t, t.Errorf, nil, err)
	diff.Test(t, t.Errorf, counter, 2)
	diff.Test(t, t.Errorf, n, uint64(18000001))
	diff.Test(t, t.Errorf, eth.EncodeHex(h), "0xd5ca78be6c6b42cf929074f502cef676372c26f8d0ba389b6f9b5d612d70f815")
}

func hash(b byte) []byte {
	res := make([]byte, 32)
	res[0] = b
	return res
}

func TestValidate(t *testing.T) {
	cases := []struct {
		start uint64
		limit uint64
		blks  []eth.Block
		want  error
	}{
		{
			1,
			1,
			[]eth.Block{
				{Header: eth.Header{Number: 1, Hash: hash(1), Parent: hash(0)}},
			},
			nil,
		},
		{
			1,
			2,
			[]eth.Block{
				{Header: eth.Header{Number: 1, Hash: hash(1), Parent: hash(0)}},
				{Header: eth.Header{Number: 2, Hash: hash(2), Parent: hash(1)}},
			},
			nil,
		},
		{
			1,
			3,
			[]eth.Block{
				{Header: eth.Header{Number: 1, Hash: hash(1), Parent: hash(0)}},
				{Header: eth.Header{Number: 2, Hash: hash(2), Parent: hash(1)}},
				{Header: eth.Header{Number: 3, Hash: hash(3), Parent: hash(2)}},
			},
			nil,
		},
		{
			1,
			3,
			[]eth.Block{
				{Header: eth.Header{Number: 1, Hash: hash(1), Parent: hash(0)}},
				{Header: eth.Header{Number: 2, Hash: hash(4), Parent: hash(3)}},
				{Header: eth.Header{Number: 3, Hash: hash(3), Parent: hash(2)}},
			},
			errors.New("test: corrupt chain segment"),
		},
	}
	for _, tc := range cases {
		diff.Test(t, t.Errorf, tc.want, validate("test", tc.start, tc.limit, tc.blks))
	}
}

func TestValidate_Blocks(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		diff.Test(t, t.Fatalf, nil, err)
		switch {
		case methodsMatch(t, body, "eth_getBlockByNumber", "eth_getBlockByNumber"):
			_, err := w.Write([]byte(`[
				{
					"result": {
						"hash": "0x95b198e154acbfc64109dfd22d8224fe927fd8dfdedfae01587674482ba4baf3",
						"number": "0x112a880"
					}
				},
				{
					"result": {
						"hash": "0xd5ca78be6c6b42cf929074f502cef676372c26f8d0ba389b6f9b5d612d70f815",
						"number": "0x112a882",
						"COMMENT": "off by one. should be 0x112a881"
					}
				}
			]`))
			diff.Test(t, t.Fatalf, nil, err)
		}
	}))
	defer ts.Close()
	var (
		ctx    = context.Background()
		c      = New(ts.URL)
		_, err = c.Get(ctx, &glf.Filter{UseBlocks: true}, 18000000, 2)
	)
	want := "getting blocks: cache get: blocks: rpc response contains invalid data. requested last: 18000001 got: 18000002"
	diff.Test(t, t.Fatalf, false, err == nil)
	diff.Test(t, t.Fatalf, want, err.Error())
}

func TestValidate_Logs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		diff.Test(t, t.Fatalf, nil, err)
		switch {
		case methodsMatch(t, body, "eth_getBlockByNumber", "eth_getLogs"):
			_, err := w.Write([]byte(`[
			{
				"result": {
					"hash": "0x95b198e154acbfc64109dfd22d8224fe927fd8dfdedfae01587674482ba4baf3",
					"number": "0x112a880"
				}
			},
			{
				"result": [
					{
						"address": "0x0000000000000000000000000000000000000000",
						"topics": [],
						"blockHash": "0x95b198e154acbfc64109dfd22d8224fe927fd8dfdedfae01587674482ba4baf3",
						"blockNumber": "0x112a880"
					},
					{
						"address": "0x0000000000000000000000000000000000000000",
						"topics": [],
						"blockHash": "0xd5ca78be6c6b42cf929074f502cef676372c26f8d0ba389b6f9b5d612d70f815",
						"blockNumber": "0x112a882",
						"COMMENT": "off by one. should be 0x112a881"
					}
				]
			}
			]`))
			diff.Test(t, t.Fatalf, nil, err)
		}
	}))
	defer ts.Close()
	var (
		ctx    = context.Background()
		c      = New(ts.URL)
		_, err = c.Get(ctx, &glf.Filter{UseLogs: true}, 18000000, 2)
	)
	tc.WantErr(t, err)
	want := "getting logs: eth_getLogs out of range block. num=18000002 start=18000000 lim=2"
	tc.WantGot(t, want, err.Error())
}

func TestValidate_Logs_NoBlocks(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		diff.Test(t, t.Fatalf, nil, err)
		switch {
		case methodsMatch(t, body, "eth_getBlockByNumber", "eth_getLogs"):
			_, err := w.Write([]byte(`[{"result": null},{"result": []}]`))
			diff.Test(t, t.Fatalf, nil, err)
		}
	}))
	defer ts.Close()
	var (
		ctx    = context.Background()
		c      = New(ts.URL)
		_, err = c.Get(ctx, &glf.Filter{UseLogs: true}, 18000000, 2)
	)
	tc.WantErr(t, err)
	const want = "getting logs: eth backend missing logs for block 18000000"
	tc.WantGot(t, want, err.Error())
}

func TestError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		diff.Test(t, t.Fatalf, nil, err)
		switch {
		case methodsMatch(t, body, "eth_getBlockByNumber"):
			_, err := w.Write([]byte(`
				[{
					"jsonrpc": "2.0",
					"id": "1",
					"error": {"code": -32012, "message": "credits"}
				}]
			`))
			diff.Test(t, t.Fatalf, nil, err)
		}
	}))
	defer ts.Close()

	var (
		ctx    = context.Background()
		c      = New(ts.URL)
		want   = "getting blocks: cache get: rpc=eth_getBlockByNumber code=-32012 msg=credits"
		_, got = c.Get(ctx, &glf.Filter{UseBlocks: true}, 1000001, 1)
	)
	diff.Test(t, t.Errorf, want, got.Error())
}

func TestGet(t *testing.T) {
	ctx := context.Background()
	const start, limit = 10, 5
	blocks, err := New("").Get(ctx, &glf.Filter{}, start, limit)
	diff.Test(t, t.Fatalf, nil, err)
	diff.Test(t, t.Fatalf, len(blocks), limit)
	diff.Test(t, t.Fatalf, blocks[0].Num(), uint64(10))
	diff.Test(t, t.Fatalf, blocks[1].Num(), uint64(11))
	diff.Test(t, t.Fatalf, blocks[2].Num(), uint64(12))
	diff.Test(t, t.Fatalf, blocks[3].Num(), uint64(13))
	diff.Test(t, t.Fatalf, blocks[4].Num(), uint64(14))
}

func TestGet_Cached(t *testing.T) {
	// reqCount is for testing concurrency.
	// We want to recreate a scenario where
	// 2 go routines attempt to call Get with
	// identical request parameters. One
	// routine will call eth_getBlockByNumber to
	// download the header and the other routine will
	// use the cached header. Once the header has been
	// downloaded, both routines will download logs.
	// Our test ensures that concurrent go routines
	// don't add duplicate tx/log data to the shared
	// block header.
	var reqCount uint64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		diff.Test(t, t.Fatalf, nil, err)
		switch {
		case methodsMatch(t, body, "eth_getBlockByNumber"):
			atomic.AddUint64(&reqCount, 1)
			_, err := w.Write([]byte(block18000000JSON))
			diff.Test(t, t.Fatalf, nil, err)
		case methodsMatch(t, body, "eth_getBlockByNumber", "eth_getLogs"):
			for ; reqCount == 0; time.Sleep(time.Second) {
			}
			_, err := w.Write([]byte(logs18000000JSON))
			diff.Test(t, t.Fatalf, nil, err)
		}
	}))
	defer ts.Close()
	var (
		ctx    = context.Background()
		c      = New(ts.URL)
		findTx = func(b *eth.Block, idx uint64) (*eth.Tx, error) {
			for i := range b.Txs {
				if b.Txs[i].Idx == eth.Uint64(idx) {
					return &b.Txs[i], nil
				}
			}
			return nil, fmt.Errorf("no tx at idx %d", idx)
		}
		getcall = func() error {
			blocks, err := c.Get(ctx, &glf.Filter{UseHeaders: true, UseLogs: true}, 18000000, 1)
			diff.Test(t, t.Errorf, nil, err)

			blocks[0].Lock()
			diff.Test(t, t.Errorf, len(blocks[0].Txs), 65)
			tx, err := findTx(&blocks[0], 0)
			diff.Test(t, t.Errorf, nil, err)
			diff.Test(t, t.Errorf, len(tx.Logs), 1)
			blocks[0].Unlock()
			return nil
		}
	)
	eg := errgroup.Group{}
	eg.Go(getcall)
	eg.Go(getcall)
	eg.Wait()
}

// Test that a block cache removes its segments after
// they've been read N times. Once N is reached, subsequent
// calls to Get should make new requests.
func TestGet_Cached_Pruned(t *testing.T) {
	var n int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		diff.Test(t, t.Fatalf, nil, err)
		switch {
		case methodsMatch(t, body, "eth_getBlockByNumber"):
			atomic.AddInt32(&n, 1)
			_, err := w.Write([]byte(block18000000JSON))
			diff.Test(t, t.Fatalf, nil, err)
		}
	}))
	defer ts.Close()
	var (
		ctx = context.Background()
		c   = New(ts.URL).WithMaxReads(2)
	)
	_, err := c.Get(ctx, &glf.Filter{UseHeaders: true}, 18000000, 1)
	diff.Test(t, t.Errorf, nil, err)
	diff.Test(t, t.Errorf, n, int32(1))
	_, err = c.Get(ctx, &glf.Filter{UseHeaders: true}, 18000000, 1)
	diff.Test(t, t.Errorf, nil, err)
	diff.Test(t, t.Errorf, n, int32(1))

	//maxreads should have been reached with last 2 calls
	_, err = c.Get(ctx, &glf.Filter{UseHeaders: true}, 18000000, 1)
	diff.Test(t, t.Errorf, nil, err)
	diff.Test(t, t.Errorf, n, int32(2))
}

func TestNoLogs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		diff.Test(t, t.Fatalf, nil, err)
		switch {
		case methodsMatch(t, body, "eth_getBlockByNumber"):
			_, err := w.Write([]byte(block1000001JSON))
			diff.Test(t, t.Fatalf, nil, err)
		case methodsMatch(t, body, "eth_getBlockByNumber", "eth_getLogs"):
			_, err := w.Write([]byte(logs1000001JSON))
			diff.Test(t, t.Fatalf, nil, err)
		}
	}))
	defer ts.Close()

	ctx := context.Background()
	c := New(ts.URL)
	blocks, err := c.Get(ctx, &glf.Filter{UseBlocks: true, UseLogs: true}, 1000001, 1)
	diff.Test(t, t.Errorf, nil, err)

	b := blocks[0]
	diff.Test(t, t.Errorf, len(b.Txs), 1)

	tx := blocks[0].Txs[0]
	diff.Test(t, t.Errorf, len(tx.Logs), 0)
}

func TestLatest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		diff.Test(t, t.Fatalf, nil, err)
		switch {
		case methodsMatch(t, body, "eth_getBlockByNumber"):
			_, err := w.Write([]byte(block18000000JSON))
			diff.Test(t, t.Fatalf, nil, err)
		case methodsMatch(t, body, "eth_getBlockByNumber", "eth_getLogs"):
			_, err := w.Write([]byte(logs18000000JSON))
			diff.Test(t, t.Fatalf, nil, err)
		}
	}))
	defer ts.Close()

	ctx := context.Background()
	c := New(ts.URL)
	blocks, err := c.Get(ctx, &glf.Filter{UseBlocks: true, UseLogs: true}, 18000000, 1)
	diff.Test(t, t.Errorf, nil, err)

	b := blocks[0]
	diff.Test(t, t.Errorf, b.Num(), uint64(18000000))
	diff.Test(t, t.Errorf, fmt.Sprintf("%.4x", b.Parent), "198723e0")
	diff.Test(t, t.Errorf, fmt.Sprintf("%.4x", b.Hash()), "95b198e1")
	diff.Test(t, t.Errorf, b.Time, eth.Uint64(1693066895))
	diff.Test(t, t.Errorf, fmt.Sprintf("%.4x", b.LogsBloom), "53f146f2")
	diff.Test(t, t.Errorf, len(b.Txs), 94)

	tx0 := blocks[0].Txs[0]
	diff.Test(t, t.Errorf, fmt.Sprintf("%.4x", tx0.Hash()), "16e19967")
	diff.Test(t, t.Errorf, fmt.Sprintf("%.4x", tx0.To), "fd14567e")
	diff.Test(t, t.Errorf, fmt.Sprintf("%s", tx0.Value.Dec()), "0")
	diff.Test(t, t.Fatalf, len(tx0.Logs), 1)

	l := blocks[0].Txs[0].Logs[0]
	diff.Test(t, t.Errorf, fmt.Sprintf("%.4x", l.Address), "fd14567e")
	diff.Test(t, t.Errorf, fmt.Sprintf("%.4x", l.Topics[0]), "b8b9c39a")
	diff.Test(t, t.Errorf, fmt.Sprintf("%x", l.Data), "01e14e6ce75f248c88ee1187bcf6c75f8aea18fbd3d927fe2d63947fcd8cb18c641569e8ee18f93c861576fe0c882e5c61a310ae8e400be6629561160d2a901f0619e35040579fa202bc3f84077a72266b2a4e744baa92b433497bc23d6aeda4")

	signer, err := tx0.Signer()
	diff.Test(t, t.Errorf, nil, err)
	diff.Test(t, t.Errorf, fmt.Sprintf("%.4x", signer), "16d5783a")

	tx3 := blocks[0].Txs[3]
	diff.Test(t, t.Errorf, fmt.Sprintf("%s", tx3.Value.Dec()), "69970000000000014")
}
