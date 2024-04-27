// JSON RPC client
package jrpc2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/indexsupply/x/eth"
	"github.com/indexsupply/x/shovel/glf"
	"github.com/indexsupply/x/wctx"

	"github.com/goccy/go-json"
	"github.com/klauspost/compress/gzhttp"
	"golang.org/x/sync/errgroup"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func New(url string) *Client {
	return &Client{
		d:       strings.Contains(url, "debug"),
		nocache: strings.Contains(url, "nocache"),
		hc: &http.Client{
			Timeout:   10 * time.Second,
			Transport: gzhttp.Transport(http.DefaultTransport),
		},
		pollDuration: time.Second,
		url:          url,
		lcache:       NumHash{maxreads: 20},
	}
}

type Client struct {
	nocache bool
	d       bool
	hc      *http.Client
	url     string
	wsurl   string

	pollDuration time.Duration

	lcache NumHash
	bcache cache
	hcache cache
}

func (c *Client) WithMaxReads(n int) *Client {
	c.lcache.maxreads = n
	return c
}

func (c *Client) WithPollDuration(d time.Duration) *Client {
	c.pollDuration = d
	return c
}

func (c *Client) WithWSURL(url string) *Client {
	c.wsurl = url
	return c
}

func (c *Client) debug(r io.Reader) io.Reader {
	if !c.d {
		return r
	}
	return io.TeeReader(r, os.Stdout)
}

type request struct {
	ID      string `json:"id"`
	Version string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  []any  `json:"params"`
}

func (c *Client) do(ctx context.Context, dest, req any) error {
	var (
		eg   errgroup.Group
		r, w = io.Pipe()
		resp *http.Response
	)
	eg.Go(func() error {
		defer w.Close()
		return json.NewEncoder(w).Encode(req)
	})
	eg.Go(func() error {
		req, err := http.NewRequest("POST", c.url, c.debug(r))
		if err != nil {
			return fmt.Errorf("unable to new request: %w", err)
		}
		req.Header.Add("content-type", "application/json")
		resp, err = c.hc.Do(req)
		if err != nil {
			return fmt.Errorf("unable to do http request: %w", err)
		}
		return nil
	})
	if err := eg.Wait(); err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		b, _ := io.ReadAll(resp.Body)
		text := strings.Map(func(r rune) rune {
			if unicode.IsPrint(r) {
				return r
			}
			return -1
		}, string(b))
		const msg = "rpc http error: %d %.100s"
		return fmt.Errorf(msg, resp.StatusCode, text)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(c.debug(resp.Body)).Decode(dest); err != nil {
		return fmt.Errorf("unable to json decode: %w", err)
	}
	wctx.CounterAdd(ctx, 1)
	return nil
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e Error) Exists() bool {
	return e.Code != 0
}

func (e Error) Error() string {
	return fmt.Sprintf("code=%d msg=%s", e.Code, e.Message)
}

type NumHash struct {
	sync.Mutex
	err      error
	once     sync.Once
	maxreads int
	nreads   int
	Num      eth.Uint64 `json:"number"`
	Hash     eth.Bytes  `json:"hash"`
}

func (nh *NumHash) error(err error) {
	nh.Lock()
	nh.nreads = 0
	nh.err = err
	nh.Unlock()
}

func (nh *NumHash) update(n eth.Uint64, h []byte) {
	nh.Lock()
	defer nh.Unlock()
	if n <= nh.Num {
		return
	}
	nh.nreads = 0
	nh.Num = n
	nh.Hash.Write(h)
}

func (nh *NumHash) get(n uint64) (uint64, []byte, bool) {
	nh.Lock()
	defer nh.Unlock()

	if err := nh.err; err != nil {
		switch {
		case errors.Is(err, net.ErrClosed), errors.Is(err, context.DeadlineExceeded):
			slog.Debug("rpc connection reset")
		default:
			slog.Debug("rpc connection error: %w", err)
		}
		nh.err = nil
		nh.once = sync.Once{}
		return 0, nil, false
	}

	if n == 0 || uint64(nh.Num) < n {
		slog.Debug("latest cache miss", "n", n, "latest", nh.Num)
		return 0, nil, false
	}

	if nh.nreads >= nh.maxreads {
		slog.Debug("expiring latest cache",
			"n", n,
			"latest", nh.Num,
			"nreads", nh.nreads,
			"maxreads", nh.maxreads,
		)
		nh.nreads = 0
		nh.Num = eth.Uint64(0)
		nh.Hash.Write([]byte{})
		return 0, nil, false
	}

	nh.nreads++
	slog.Debug("latest cache hit",
		"n", n,
		"latest", nh.Num,
		"nreads", nh.nreads,
	)
	h := make([]byte, 32)
	copy(h, nh.Hash)
	return uint64(nh.Num), h, true
}

func (c *Client) wsListen(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	wsc, _, err := websocket.Dial(ctx, c.wsurl, nil)
	if err != nil {
		c.lcache.error(fmt.Errorf("ws dial %q: %w", c.wsurl, err))
		return
	}
	err = wsjson.Write(ctx, wsc, request{
		ID:      "1",
		Version: "2.0",
		Method:  "eth_subscribe",
		Params:  []any{"newHeads"},
	})
	if err != nil {
		c.lcache.error(fmt.Errorf("ws write %q: %w", c.wsurl, err))
		return
	}
	res := struct {
		Error `json:"error"`
		P     struct {
			R NumHash `json:"result"`
		} `json:"params"`
	}{}
	for {
		if err := wsjson.Read(ctx, wsc, &res); err != nil {
			c.lcache.error(fmt.Errorf("ws read %q: %w", c.wsurl, err))
			return
		}
		slog.Debug("websocket newHeads",
			"n", res.P.R.Num,
			"h", fmt.Sprintf("%.4x", res.P.R.Hash),
		)
		c.lcache.update(res.P.R.Num, res.P.R.Hash)
	}
}

func (c *Client) httpPoll(ctx context.Context) {
	var (
		ticker = time.NewTicker(c.pollDuration)
		hresp  = headerResp{}
	)
	defer ticker.Stop()
	for range ticker.C {
		err := c.do(ctx, &hresp, request{
			ID:      "1",
			Version: "2.0",
			Method:  "eth_getBlockByNumber",
			Params:  []any{"latest", false},
		})
		if err != nil {
			c.lcache.error(err)
			return
		}
		if hresp.Error.Exists() {
			const tag = "eth_getBlockByNumber/latest"
			c.lcache.error(fmt.Errorf("rpc=%s %w", tag, hresp.Error))
			return
		}
		slog.Debug("http poll",
			"n", hresp.Number,
			"h", fmt.Sprintf("%.4x", hresp.Hash),
		)
		c.lcache.update(hresp.Number, hresp.Hash)
	}
}

// Returns the latest block number/hash greater than n.
// If n is lower than the cached block number,
// returns the cached value; otherwise, fetches the
// latest block. Caching is based on comparing n
// with the cached block number, not on time/LRU methods.
//
// When n is 0, Latest always fetches the latest block
// rather than using the cached value,
// bypassing the caching mechanism.
func (c *Client) Latest(ctx context.Context, n uint64) (uint64, []byte, error) {
	c.lcache.once.Do(func() {
		switch {
		case len(c.wsurl) > 0:
			slog.Debug("jrpc2 ws listening")
			go c.wsListen(context.Background())
		default:
			slog.Debug("jrpc2 http polling")
			go c.httpPoll(context.Background())
		}
	})
	if n, h, ok := c.lcache.get(n); ok {
		return n, h, nil
	}

	hresp := headerResp{}
	err := c.do(ctx, &hresp, request{
		ID:      "1",
		Version: "2.0",
		Method:  "eth_getBlockByNumber",
		Params:  []any{"latest", false},
	})
	if err != nil {
		return 0, nil, fmt.Errorf("unable request latest: %w", err)
	}
	if hresp.Error.Exists() {
		const tag = "eth_getBlockByNumber/latest"
		return 0, nil, fmt.Errorf("rpc=%s %w", tag, hresp.Error)
	}
	slog.Debug("http get latest",
		"n", hresp.Number,
		"h", fmt.Sprintf("%.4x", hresp.Hash),
	)
	c.lcache.update(hresp.Number, hresp.Hash)
	return uint64(hresp.Number), hresp.Hash, nil
}

func (c *Client) Hash(ctx context.Context, n uint64) ([]byte, error) {
	hresp := headerResp{}
	err := c.do(ctx, &hresp, request{
		ID:      "1",
		Version: "2.0",
		Method:  "eth_getBlockByNumber",
		Params:  []any{"0x" + strconv.FormatUint(n, 16), true},
	})
	if err != nil {
		return nil, fmt.Errorf("unable request hash: %w", err)
	}
	if hresp.Error.Exists() {
		const tag = "eth_getBlockByNumber/hash"
		return nil, fmt.Errorf("rpc=%s %w", tag, hresp.Error)
	}
	return hresp.Hash, nil
}

type key struct {
	a, b uint64
}

type (
	blockmap map[uint64]*eth.Block
)

func (c *Client) Get(
	ctx context.Context,
	filter *glf.Filter,
	start, limit uint64,
) ([]eth.Block, error) {
	var (
		blocks []eth.Block
		err    error
	)
	switch {
	case filter.UseBlocks:
		blocks, err = c.bcache.get(c.nocache, ctx, start, limit, c.blocks)
		if err != nil {
			return nil, fmt.Errorf("getting blocks: %w", err)
		}
	case filter.UseHeaders:
		blocks, err = c.hcache.get(c.nocache, ctx, start, limit, c.headers)
		if err != nil {
			return nil, fmt.Errorf("getting blocks: %w", err)
		}
	default:
		for i := uint64(0); i < limit; i++ {
			blocks = append(blocks, eth.Block{
				Header: eth.Header{
					Number: eth.Uint64(start + i),
				},
			})
		}
	}

	bm := make(blockmap)
	for i := range blocks {
		bm[blocks[i].Num()] = &blocks[i]
	}

	switch {
	case filter.UseReceipts:
		if err := c.receipts(ctx, bm, start, limit); err != nil {
			return nil, fmt.Errorf("getting receipts: %w", err)
		}
	case filter.UseLogs:
		if err := c.logs(ctx, filter, bm, start, limit); err != nil {
			return nil, fmt.Errorf("getting logs: %w", err)
		}
	case filter.UseTraces:
		if err := c.traces(ctx, bm, start, limit); err != nil {
			return nil, fmt.Errorf("getting traces: %w", err)
		}
	}
	return blocks, validate("Get", start, limit, blocks)
}

type blockResp struct {
	Error      `json:"error"`
	*eth.Block `json:"result"`
}

type segment struct {
	sync.Mutex
	done bool
	d    []eth.Block
}

type cache struct {
	sync.Mutex
	segments map[key]*segment
}

type getter func(ctx context.Context, start, limit uint64) ([]eth.Block, error)

func (c *cache) prune() {
	const size = 5
	if len(c.segments) <= size {
		return
	}
	var keys []key
	for k := range c.segments {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].a > keys[j].a
	})
	for i := range keys[size:] {
		delete(c.segments, keys[size+i])
	}
}

func (c *cache) get(nocache bool, ctx context.Context, start, limit uint64, f getter) ([]eth.Block, error) {
	if nocache {
		return f(ctx, start, limit)
	}
	c.Lock()
	if c.segments == nil {
		c.segments = make(map[key]*segment)
	}
	seg, ok := c.segments[key{start, limit}]
	if !ok {
		seg = &segment{}
		c.segments[key{start, limit}] = seg
	}
	c.prune()
	c.Unlock()

	seg.Lock()
	defer seg.Unlock()
	if seg.done {
		return seg.d, nil
	}

	blocks, err := f(ctx, start, limit)
	if err != nil {
		return nil, fmt.Errorf("cache get: %w", err)
	}

	seg.d = blocks
	seg.done = true
	return seg.d, nil
}

func (c *Client) blocks(ctx context.Context, start, limit uint64) ([]eth.Block, error) {
	var (
		reqs   = make([]request, limit)
		resps  = make([]blockResp, limit)
		blocks = make([]eth.Block, limit)
	)
	for i := uint64(0); i < limit; i++ {
		reqs[i] = request{
			ID:      "1",
			Version: "2.0",
			Method:  "eth_getBlockByNumber",
			Params:  []any{eth.EncodeUint64(start + i), true},
		}
		resps[i].Block = &blocks[i]
	}
	err := c.do(ctx, &resps, reqs)
	if err != nil {
		return nil, fmt.Errorf("requesting blocks: %w", err)
	}
	for i := range resps {
		if resps[i].Error.Exists() {
			const tag = "eth_getBlockByNumber"
			return nil, fmt.Errorf("rpc=%s %w", tag, resps[i].Error)
		}
	}
	return blocks, validate("blocks", start, limit, blocks)
}

func validate(caller string, start, limit uint64, blocks []eth.Block) error {
	if len(blocks) == 0 {
		return fmt.Errorf("%s: no blocks", caller)
	}

	first, last := blocks[0], blocks[len(blocks)-1]
	if uint64(first.Num()) != start {
		const tag = "%s: rpc response contains invalid data. requested first: %d got: %d"
		return fmt.Errorf(tag, caller, start, first.Num())
	}
	if uint64(last.Num()) != start+limit-1 {
		const tag = "%s: rpc response contains invalid data. requested last: %d got: %d"
		return fmt.Errorf(tag, caller, start+limit-1, last.Num())
	}

	// some rpc responses will not return a parent hash
	// so there is nothing we can do to validate the hash
	// chain
	if len(blocks) <= 1 || len(blocks[0].Header.Parent) < 32 {
		return nil
	}
	for i := 1; i < len(blocks); i++ {
		prev, curr := blocks[i-1], blocks[i]
		if !bytes.Equal(curr.Header.Parent, prev.Hash()) {
			slog.Error("rpc response contains invalid data",
				"num", prev.Num(),
				"hash", fmt.Sprintf("%.4x", prev.Header.Hash),
				"next-num", curr.Num(),
				"next-parent", fmt.Sprintf("%.4x", curr.Header.Parent),
				"next-hash", fmt.Sprintf("%.4x", curr.Header.Hash),
			)
			return fmt.Errorf("%s: corrupt chain segment", caller)
		}
	}
	return nil
}

type headerResp struct {
	Error       `json:"error"`
	*eth.Header `json:"result"`
}

func (c *Client) headers(ctx context.Context, start, limit uint64) ([]eth.Block, error) {
	var (
		t0     = time.Now()
		reqs   = make([]request, limit)
		resps  = make([]headerResp, limit)
		blocks = make([]eth.Block, limit)
	)
	for i := uint64(0); i < limit; i++ {
		reqs[i] = request{
			ID:      "1",
			Version: "2.0",
			Method:  "eth_getBlockByNumber",
			Params:  []any{eth.EncodeUint64(start + i), false},
		}
		resps[i].Header = &blocks[i].Header
	}
	err := c.do(ctx, &resps, reqs)
	if err != nil {
		return nil, fmt.Errorf("requesting headers: %w", err)
	}
	for i := range resps {
		if resps[i].Error.Exists() {
			const tag = "eth_getBlockByNumber/headers"
			return nil, fmt.Errorf("rpc=%s %w", tag, resps[i].Error)
		}
	}
	slog.Debug("http get headers",
		"start", start,
		"limit", limit,
		"latency", time.Since(t0),
	)
	return blocks, validate("headers", start, limit, blocks)
}

type receiptResult struct {
	BlockHash eth.Bytes  `json:"blockHash"`
	BlockNum  eth.Uint64 `json:"blockNumber"`
	TxHash    eth.Bytes  `json:"transactionHash"`
	TxIdx     eth.Uint64 `json:"transactionIndex"`
	TxType    eth.Byte   `json:"type"`
	TxFrom    eth.Bytes  `json:"from"`
	TxTo      eth.Bytes  `json:"to"`
	Status    eth.Byte   `json:"status"`
	GasUsed   eth.Uint64 `json:"gasUsed"`
	Logs      eth.Logs   `json:"logs"`
}

type receiptResp struct {
	Error  `json:"error"`
	Result []receiptResult `json:"result"`
}

func (c *Client) receipts(ctx context.Context, bm blockmap, start, limit uint64) error {
	var (
		reqs  = make([]request, limit)
		resps = make([]receiptResp, limit)
	)
	for i := uint64(0); i < limit; i++ {
		reqs[i] = request{
			ID:      "1",
			Version: "2.0",
			Method:  "eth_getBlockReceipts",
			Params:  []any{eth.EncodeUint64(start + i)},
		}
	}
	err := c.do(ctx, &resps, reqs)
	if err != nil {
		return fmt.Errorf("requesting receipts: %w", err)
	}
	for i := range resps {
		if resps[i].Error.Exists() {
			const tag = "eth_getBlockReceipts"
			return fmt.Errorf("rpc=%s %w", tag, resps[i].Error)
		}
	}
	for i := range resps {
		if len(resps[i].Result) == 0 {
			return fmt.Errorf("no rpc error but empty result")
		}
		b, ok := bm[uint64(resps[i].Result[0].BlockNum)]
		if !ok {
			return fmt.Errorf("block not found")
		}
		b.Header.Hash.Write(resps[i].Result[0].BlockHash)
		for j := range resps[i].Result {
			tx := b.Tx(uint64(resps[i].Result[j].TxIdx))
			tx.PrecompHash.Write(resps[i].Result[j].TxHash)
			tx.Type.Write(byte(resps[i].Result[j].TxType))
			tx.From.Write(resps[i].Result[j].TxFrom)
			tx.To.Write(resps[i].Result[j].TxTo)
			tx.Status.Write(byte(resps[i].Result[j].Status))
			tx.GasUsed = resps[i].Result[j].GasUsed
			tx.Logs = make([]eth.Log, len(resps[i].Result[j].Logs))
			copy(tx.Logs, resps[i].Result[j].Logs)
		}
	}
	return nil
}

type logResult struct {
	*eth.Log
	BlockHash eth.Bytes  `json:"blockHash"`
	BlockNum  eth.Uint64 `json:"blockNumber"`
	TxHash    eth.Bytes  `json:"transactionHash"`
	TxIdx     eth.Uint64 `json:"transactionIndex"`
	Removed   bool       `json:"removed"`
}

type logResp struct {
	Error  `json:"error"`
	Result []logResult `json:"result"`
}

func (c *Client) logs(ctx context.Context, filter *glf.Filter, bm blockmap, start, limit uint64) error {
	var (
		t0 = time.Now()
		lf = struct {
			From    string     `json:"fromBlock"`
			To      string     `json:"toBlock"`
			Address []string   `json:"address"`
			Topics  [][]string `json:"topics"`
		}{
			From:    eth.EncodeUint64(start),
			To:      eth.EncodeUint64(start + limit - 1),
			Address: filter.Addresses(),
			Topics:  filter.Topics(),
		}
		lresp = logResp{}
	)
	err := c.do(ctx, &lresp, request{
		ID:      "1",
		Version: "2.0",
		Method:  "eth_getLogs",
		Params:  []any{lf},
	})
	if err != nil {
		return fmt.Errorf("making logs request: %w", err)
	}
	if lresp.Error.Exists() {
		return fmt.Errorf("rpc=%s %w", "eth_getLogs", lresp.Error)
	}
	var logsByTx = map[key][]logResult{}
	for i := range lresp.Result {
		k := key{uint64(lresp.Result[i].BlockNum), uint64(lresp.Result[i].TxIdx)}
		if logs, ok := logsByTx[k]; ok {
			logsByTx[k] = append(logs, lresp.Result[i])
			continue
		}
		logsByTx[k] = []logResult{lresp.Result[i]}
	}

	for k, logs := range logsByTx {
		b, ok := bm[k.a]
		if !ok {
			return fmt.Errorf("block not found")
		}
		b.Header.Hash.Write(logs[0].BlockHash)
		tx := b.Tx(k.b)
		tx.PrecompHash.Write(logs[0].TxHash)
		for i := range logs {
			tx.Logs.Add(logs[i].Log)
		}
	}
	slog.Debug("http get logs",
		"start", start,
		"limit", limit,
		"latency", time.Since(t0),
	)
	return nil
}

type traceBlockResult struct {
	BlockHash eth.Bytes       `json:"blockHash"`
	BlockNum  uint64          `json:"blockNumber"`
	TxHash    eth.Bytes       `json:"transactionHash"`
	TxIdx     uint64          `json:"transactionPosition"`
	Action    eth.TraceAction `json:"action"`
}

type traceBlockResp struct {
	Error  `json:"error"`
	Result []traceBlockResult `json:"result"`
}

func (c *Client) traces(ctx context.Context, bm blockmap, start, limit uint64) error {
	t0 := time.Now()
	for i := uint64(0); i < limit; i++ {
		res := traceBlockResp{}
		req := request{
			ID:      "1",
			Version: "2.0",
			Method:  "trace_block",
			Params:  []any{eth.EncodeUint64(start + i)},
		}
		err := c.do(ctx, &res, req)
		if err != nil {
			return fmt.Errorf("requesting traces: %w", err)
		}
		if res.Error.Exists() {
			const tag = "trace_block"
			return fmt.Errorf("rpc=%s %w", tag, res.Error)
		}
		if len(res.Result) == 0 {
			return fmt.Errorf("no rpc error but empty result")
		}
		block, ok := bm[res.Result[0].BlockNum]
		if !ok {
			return fmt.Errorf("missing block in block map")
		}
		block.Header.Hash.Write(res.Result[0].BlockHash)

		var tracesByTx = map[key][]traceBlockResult{}
		for i := range res.Result {
			k := key{block.Num(), uint64(res.Result[i].TxIdx)}
			if traces, ok := tracesByTx[k]; ok {
				tracesByTx[k] = append(traces, res.Result[i])
				continue
			}
			tracesByTx[k] = []traceBlockResult{res.Result[i]}
		}
		for k, traces := range tracesByTx {
			tx := block.Tx(k.b)
			tx.PrecompHash.Write(traces[0].TxHash)
			tx.TraceActions = make([]eth.TraceAction, len(traces))
			for i := range traces {
				ta := traces[i].Action
				ta.Idx = uint64(i)
				tx.TraceActions[i] = ta
			}
		}
	}
	slog.Debug("http get traces",
		"start", start,
		"limit", limit,
		"latency", time.Since(t0),
	)
	return nil
}
