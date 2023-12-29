// JSON RPC client
package jrpc2

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/indexsupply/x/eth"
	"github.com/indexsupply/x/shovel/glf"
)

type key struct {
	b uint64
	t uint64
}

type lookupCache struct {
	b map[uint64]*eth.Block
	t map[key]*eth.Tx
	l map[key][]logResult
}

func New(url string, filter glf.Filter) *Client {
	return &Client{
		d:      strings.Contains(url, "debug"),
		filter: filter,
		hc:     &http.Client{},
		url:    url,
		lookup: lookupCache{
			b: map[uint64]*eth.Block{},
			t: map[key]*eth.Tx{},
			l: map[key][]logResult{},
		},
	}
}

type Client struct {
	d      bool
	filter glf.Filter
	hc     *http.Client
	url    string
	lookup lookupCache
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

func (c *Client) do(req any) (io.ReadCloser, error) {
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
		return nil, err
	}
	return resp.Body, nil
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

func (c *Client) Latest() (uint64, []byte, error) {
	resp, err := c.do(request{
		ID:      "1",
		Version: "2.0",
		Method:  "eth_getBlockByNumber",
		Params:  []any{"latest", true},
	})
	if err != nil {
		return 0, nil, fmt.Errorf("unable request hash: %w", err)
	}
	defer resp.Close()
	bresp := blockResp{Block: &eth.Block{}}
	if err := json.NewDecoder(c.debug(resp)).Decode(&bresp); err != nil {
		return 0, nil, fmt.Errorf("unable to decode json into response: %w", err)
	}
	if bresp.Error.Exists() {
		const tag = "eth_getBlockByNumber/latest"
		return 0, nil, fmt.Errorf("rpc=%s %w", tag, bresp.Error)
	}
	return bresp.Num(), bresp.Hash(), nil
}

func (c *Client) Hash(n uint64) ([]byte, error) {
	resp, err := c.do(request{
		ID:      "1",
		Version: "2.0",
		Method:  "eth_getBlockByNumber",
		Params:  []any{"0x" + strconv.FormatUint(n, 16), true},
	})
	if err != nil {
		return nil, fmt.Errorf("unable request hash: %w", err)
	}
	defer resp.Close()
	bresp := blockResp{Block: &eth.Block{}}
	if err := json.NewDecoder(resp).Decode(&bresp); err != nil {
		return nil, fmt.Errorf("unable to decode json into response: %w", err)
	}
	if bresp.Error.Exists() {
		const tag = "eth_getBlockByNumber/hash"
		return nil, fmt.Errorf("rpc=%s %w", tag, bresp.Error)
	}
	return bresp.Hash(), nil
}

func (c *Client) LoadBlocks(_ [][]byte, blocks []eth.Block) error {
	clear(c.lookup.b)
	clear(c.lookup.t)
	clear(c.lookup.l)
	for i := range blocks {
		c.lookup.b[blocks[i].Num()] = &blocks[i]
	}

	switch {
	case c.filter.UseBlocks:
		if err := c.blocks(blocks); err != nil {
			return fmt.Errorf("getting blocks: %w", err)
		}
	case c.filter.UseHeaders:
		if err := c.headers(blocks); err != nil {
			return fmt.Errorf("getting headers: %w", err)
		}
	}
	switch {
	case c.filter.UseReceipts:
		if err := c.receipts(blocks); err != nil {
			return fmt.Errorf("getting receipts: %w", err)
		}

	case c.filter.UseLogs:
		if err := c.logs(blocks); err != nil {
			return fmt.Errorf("getting logs: %w", err)
		}
	}
	return nil
}

type blockResp struct {
	Error      `json:"error"`
	*eth.Block `json:"result"`
}

func (c *Client) blocks(blocks []eth.Block) error {
	var (
		reqs  = make([]request, len(blocks))
		resps = make([]blockResp, len(blocks))
	)
	for i := range blocks {
		n := "0x" + strconv.FormatUint(blocks[i].Num(), 16)
		reqs[i] = request{
			ID:      "1",
			Version: "2.0",
			Method:  "eth_getBlockByNumber",
			Params:  []any{n, true},
		}
		resps[i].Block = &blocks[i]
	}
	resp, err := c.do(reqs)
	if err != nil {
		return fmt.Errorf("requesting blocks: %w", err)
	}
	defer resp.Close()
	if err := json.NewDecoder(c.debug(resp)).Decode(&resps); err != nil {
		return fmt.Errorf("unable to decode json into response: %w", err)
	}
	for i := range resps {
		if resps[i].Error.Exists() {
			return fmt.Errorf("rpc=%s %w", "eth_getBlockByNumber", resps[i].Error)
		}
	}
	for i := range blocks {
		for j := range blocks[i].Txs {
			t := &blocks[i].Txs[j]
			k := key{
				b: blocks[i].Num(),
				t: uint64(t.Idx),
			}
			c.lookup.t[k] = t
		}
	}
	return nil
}

type headerResp struct {
	Error       `json:"error"`
	*eth.Header `json:"result"`
}

func (c *Client) headers(blocks []eth.Block) error {
	var (
		reqs  = make([]request, len(blocks))
		resps = make([]headerResp, len(blocks))
	)
	for i := range blocks {
		reqs[i] = request{
			ID:      "1",
			Version: "2.0",
			Method:  "eth_getBlockByNumber",
			Params:  []any{eth.EncodeUint64(blocks[i].Num()), false},
		}
		resps[i].Header = &blocks[i].Header
	}
	resp, err := c.do(reqs)
	if err != nil {
		return fmt.Errorf("requesting headers: %w", err)
	}
	defer resp.Close()
	if err := json.NewDecoder(c.debug(resp)).Decode(&resps); err != nil {
		return fmt.Errorf("unable to decode json into response: %w", err)
	}
	for i := range resps {
		if resps[i].Error.Exists() {
			const tag = "eth_getBlockByNumber/headers"
			return fmt.Errorf("rpc=%s %w", tag, resps[i].Error)
		}
	}
	return nil
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

func (c *Client) receipts(blocks []eth.Block) error {
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
	resp, err := c.do(reqs)
	if err != nil {
		return fmt.Errorf("requesting blocks: %w", err)
	}
	defer resp.Close()
	if err := json.NewDecoder(c.debug(resp)).Decode(&resps); err != nil {
		return fmt.Errorf("unable to decode json into response: %w", err)
	}
	for i := range resps {
		if resps[i].Error.Exists() {
			const tag = "eth_getBlockReceipts"
			return fmt.Errorf("rpc=%s %w", tag, resps[i].Error)
		}
	}
	for i := range resps {
		b, ok := c.lookup.b[uint64(resps[i].Result[0].BlockNum)]
		if !ok {
			return fmt.Errorf("block not found")
		}
		b.Header.Hash.Write(resps[i].Result[0].BlockHash)
		for j := range resps[i].Result {
			k := key{
				b: b.Num(),
				t: uint64(resps[i].Result[j].TxIdx),
			}
			if tx, ok := c.lookup.t[k]; ok {
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

func (c *Client) logs(blocks []eth.Block) error {
	lf := struct {
		From   string   `json:"fromBlock"`
		To     string   `json:"toBlock"`
		Topics []string `json:"topics"`
	}{
		From: "0x" + strconv.FormatUint(blocks[0].Num(), 16),
		To:   "0x" + strconv.FormatUint(blocks[len(blocks)-1].Num(), 16),
	}
	resp, err := c.do(request{
		ID:      "1",
		Version: "2.0",
		Method:  "eth_getLogs",
		Params:  []any{lf},
	})
	if err != nil {
		return fmt.Errorf("making logs request: %w", err)
	}
	defer resp.Close()
	lresp := logResp{}
	if err := json.NewDecoder(c.debug(resp)).Decode(&lresp); err != nil {
		return fmt.Errorf("unable to decode json into response: %w", err)
	}
	if lresp.Error.Exists() {
		return fmt.Errorf("rpc=%s %w", "eth_getLogs", lresp.Error)
	}
	slices.SortFunc(lresp.Result, func(a, b logResult) int {
		return cmp.Compare(a.Log.Idx, b.Log.Idx)
	})
	for i := range lresp.Result {
		k := key{
			b: uint64(lresp.Result[i].BlockNum),
			t: uint64(lresp.Result[i].TxIdx),
		}
		if logs, ok := c.lookup.l[k]; ok {
			c.lookup.l[k] = append(logs, lresp.Result[i])
			continue
		}
		c.lookup.l[k] = []logResult{lresp.Result[i]}
	}
	for k, logs := range c.lookup.l {
		b, ok := c.lookup.b[k.b]
		if !ok {
			return fmt.Errorf("block not found")
		}
		if tx, ok := c.lookup.t[k]; ok {
			for i := range logs {
				tx.Logs = append(tx.Logs, *logs[i].Log)
			}
			continue
		}
		tx := eth.Tx{}
		tx.Idx = eth.Uint64(k.t)
		tx.PrecompHash.Write(logs[0].TxHash)
		tx.Logs = make([]eth.Log, 0, len(logs))
		for i := range logs {
			tx.Logs = append(tx.Logs, *logs[i].Log)
		}
		b.Header.Hash.Write(logs[0].BlockHash)
		b.Header.Number = eth.Uint64(logs[0].BlockNum)
		b.Txs = append(b.Txs, tx)
	}
	return nil
}
