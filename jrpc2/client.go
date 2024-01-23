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

	"github.com/goccy/go-json"
	"github.com/klauspost/compress/gzhttp"
	"golang.org/x/sync/errgroup"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func New(url string) *Client {
	return &Client{
		d: strings.Contains(url, "debug"),
		hc: &http.Client{
			Timeout:   10 * time.Second,
			Transport: gzhttp.Transport(http.DefaultTransport),
		},
		url: url,
	}
}

type Client struct {
	d   bool
	hc  *http.Client
	url string

	wsurl string
	wserr error
	once  sync.Once

	lcache NumHash
	bcache cache
	hcache cache
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

func (c *Client) do(dest, req any) error {
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
	Num  eth.Uint64 `json:"number"`
	Hash eth.Bytes  `json:"hash"`
}

func (c *Client) listen(ctx context.Context) {
	if len(c.wsurl) == 0 {
		slog.InfoContext(ctx, "no wsurl set. skipping websocket")
		return
	}

	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	wsc, _, err := websocket.Dial(ctx, c.wsurl, nil)
	if err != nil {
		c.wserr = fmt.Errorf("ws dial %q: %w", c.wsurl, err)
		return
	}
	err = wsjson.Write(ctx, wsc, request{
		ID:      "1",
		Version: "2.0",
		Method:  "eth_subscribe",
		Params:  []any{"newHeads"},
	})
	if err != nil {
		c.wserr = fmt.Errorf("ws write %q: %w", c.wsurl, err)
		return
	}
	res := struct {
		Error  `json:"error"`
		Params struct {
			Result NumHash `json:"result"`
		} `json:"params"`
	}{}
	for {
		if err := wsjson.Read(ctx, wsc, &res); err != nil {
			c.wserr = fmt.Errorf("ws read %q: %w", c.wsurl, err)
			return
		}
		var (
			num  = res.Params.Result.Num
			hash = res.Params.Result.Hash
		)
		c.lcache.Lock()
		if c.lcache.Num >= num {
			c.lcache.Unlock()
			continue
		}
		c.lcache.Num = num
		c.lcache.Hash.Write(hash)
		c.lcache.Unlock()
		slog.Debug("websocket newHeads",
			"n", num,
			"h", fmt.Sprintf("%.4x", hash),
		)
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
func (c *Client) Latest(n uint64) (uint64, []byte, error) {
	c.lcache.Lock()
	defer c.lcache.Unlock()

	c.once.Do(func() {
		go c.listen(context.Background())
	})
	if err := c.wserr; err != nil {
		c.wserr = nil
		c.once = sync.Once{}
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			slog.Debug("resetting ws connection. timeout")
		case errors.Is(err, net.ErrClosed):
			slog.Debug("resetting ws connection. closed")
		default:
			return 0, nil, fmt.Errorf("wserr: %w", err)
		}
	}

	if n > 0 && n < uint64(c.lcache.Num) {
		slog.Debug("latest cache hit",
			"n", n,
			"latest", c.lcache.Num,
		)
		h := make([]byte, 32)
		copy(h, c.lcache.Hash)
		return uint64(c.lcache.Num), h, nil
	}

	hresp := headerResp{}
	err := c.do(&hresp, request{
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

	slog.Debug("latest cache miss",
		"n", n,
		"previous", c.lcache.Num,
		"latest", hresp.Number,
	)

	if hresp.Number > c.lcache.Num {
		c.lcache.Num = hresp.Number
		c.lcache.Hash.Write(hresp.Hash)
	}

	h := make([]byte, 32)
	copy(h, hresp.Hash)
	return uint64(c.lcache.Num), h, nil
}

func (c *Client) Hash(n uint64) ([]byte, error) {
	hresp := headerResp{}
	err := c.do(&hresp, request{
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
	txmap    map[key]*eth.Tx
)

func (c *Client) Get(filter *glf.Filter, start, limit uint64) ([]eth.Block, error) {
	var (
		blocks []eth.Block
		err    error
	)
	switch {
	case filter.UseBlocks:
		blocks, err = c.bcache.get(start, limit, c.blocks)
		if err != nil {
			return nil, fmt.Errorf("getting blocks: %w", err)
		}
	case filter.UseHeaders:
		blocks, err = c.hcache.get(start, limit, c.headers)
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

	bm, tm := make(blockmap), make(txmap)
	for i := range blocks {
		bm[blocks[i].Num()] = &blocks[i]
		for j := range blocks[i].Txs {
			t := &blocks[i].Txs[j]
			k := key{blocks[i].Num(), uint64(t.Idx)}
			tm[k] = t
		}
	}

	switch {
	case filter.UseReceipts:
		if err := c.receipts(bm, tm, blocks); err != nil {
			return nil, fmt.Errorf("getting receipts: %w", err)
		}
	case filter.UseLogs:
		if err := c.logs(filter, bm, tm, blocks); err != nil {
			return nil, fmt.Errorf("getting logs: %w", err)
		}
	}
	return blocks, nil
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

type getter func(start, limit uint64) ([]eth.Block, error)

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

func (c *cache) get(start, limit uint64, f getter) ([]eth.Block, error) {
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

	blocks, err := f(start, limit)
	if err != nil {
		return nil, fmt.Errorf("cache get: %w", err)
	}

	seg.d = blocks
	seg.done = true
	return seg.d, nil
}

func (c *Client) blocks(start, limit uint64) ([]eth.Block, error) {
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
	err := c.do(&resps, reqs)
	if err != nil {
		return nil, fmt.Errorf("requesting blocks: %w", err)
	}
	for i := range resps {
		if resps[i].Error.Exists() {
			const tag = "eth_getBlockByNumber"
			return nil, fmt.Errorf("rpc=%s %w", tag, resps[i].Error)
		}
	}
	return blocks, validate(start, limit, blocks)
}

func validate(start, limit uint64, blocks []eth.Block) error {
	if len(blocks) == 0 {
		return fmt.Errorf("no blocks")
	}

	first, last := blocks[0], blocks[len(blocks)-1]
	if uint64(first.Num()) != start {
		const tag = "rpc response contains invalid data. requested first: %d got: %d"
		return fmt.Errorf(tag, start, first.Num())
	}
	if uint64(last.Num()) != start+limit-1 {
		const tag = "rpc response contains invalid data. requested last: %d got: %d"
		return fmt.Errorf(tag, start+limit-1, last.Num())
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
			return fmt.Errorf("corrupt chain segment")
		}
	}
	return nil
}

type headerResp struct {
	Error       `json:"error"`
	*eth.Header `json:"result"`
}

func (c *Client) headers(start, limit uint64) ([]eth.Block, error) {
	var (
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
	err := c.do(&resps, reqs)
	if err != nil {
		return nil, fmt.Errorf("requesting headers: %w", err)
	}
	for i := range resps {
		if resps[i].Error.Exists() {
			const tag = "eth_getBlockByNumber/headers"
			return nil, fmt.Errorf("rpc=%s %w", tag, resps[i].Error)
		}
	}
	return blocks, nil
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

func (c *Client) receipts(bm blockmap, tm txmap, blocks []eth.Block) error {
	var (
		reqs  = make([]request, len(blocks))
		resps = make([]receiptResp, len(blocks))
	)
	for i := range blocks {
		reqs[i] = request{
			ID:      "1",
			Version: "2.0",
			Method:  "eth_getBlockReceipts",
			Params:  []any{eth.EncodeUint64(blocks[i].Num())},
		}
	}
	err := c.do(&resps, reqs)
	if err != nil {
		return fmt.Errorf("requesting blocks: %w", err)
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
			k := key{b.Num(), uint64(resps[i].Result[j].TxIdx)}
			if tx, ok := tm[k]; ok {
				tx.Status.Write(byte(resps[i].Result[j].Status))
				tx.GasUsed = resps[i].Result[j].GasUsed
				tx.Logs = make([]eth.Log, len(resps[i].Result[j].Logs))
				copy(tx.Logs, resps[i].Result[j].Logs)
				continue
			}

			tx := eth.Tx{}
			tx.Idx = resps[i].Result[j].TxIdx
			tx.PrecompHash.Write(resps[i].Result[j].TxHash)
			tx.Type.Write(byte(resps[i].Result[j].TxType))
			tx.From.Write(resps[i].Result[j].TxFrom)
			tx.To.Write(resps[i].Result[j].TxTo)
			tx.Status.Write(byte(resps[i].Result[j].Status))
			tx.GasUsed = resps[i].Result[j].GasUsed
			tx.Logs = make([]eth.Log, len(resps[i].Result[j].Logs))
			copy(tx.Logs, resps[i].Result[j].Logs)
			b.Txs = append(b.Txs, tx)
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

func (c *Client) logs(filter *glf.Filter, bm blockmap, tm txmap, blocks []eth.Block) error {
	var (
		lf = struct {
			From    string     `json:"fromBlock"`
			To      string     `json:"toBlock"`
			Address []string   `json:"address"`
			Topics  [][]string `json:"topics"`
		}{
			From:    eth.EncodeUint64(blocks[0].Num()),
			To:      eth.EncodeUint64(blocks[len(blocks)-1].Num()),
			Address: filter.Addresses(),
			Topics:  filter.Topics(),
		}
		lresp = logResp{}
	)
	err := c.do(&lresp, request{
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
		if tx, ok := tm[k]; ok {
			for i := range logs {
				var found bool
				for j := range tx.Logs {
					if tx.Logs[j].Idx == logs[i].Log.Idx {
						found = true
					}
				}
				if !found {
					tx.Logs = append(tx.Logs, *logs[i].Log)
				}
			}
			continue
		}
		tx := eth.Tx{}
		tx.Idx = eth.Uint64(k.b)
		tx.PrecompHash.Write(logs[0].TxHash)
		tx.Logs = make([]eth.Log, 0, len(logs))
		for i := range logs {
			tx.Logs.Add(logs[i].Log)
		}
		b.Header.Hash.Write(logs[0].BlockHash)
		b.Header.Number = eth.Uint64(logs[0].BlockNum)
		b.Txs = append(b.Txs, tx)
	}
	return nil
}
