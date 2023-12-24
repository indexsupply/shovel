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

	"github.com/indexsupply/x/eth"
	"github.com/indexsupply/x/shovel/glf"

	"golang.org/x/sync/errgroup"
)

func New(url string, filter glf.Filter) *Client {
	return &Client{
		d:      strings.Contains(url, "debug"),
		hc:     &http.Client{},
		url:    url,
		filter: filter,
	}
}

type Client struct {
	d      bool
	url    string
	hc     *http.Client
	filter glf.Filter
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

func check(e Error) error {
	if e.Code != 0 {
		return fmt.Errorf("rpc error: %d %s", e.Code, e.Message)
	}
	return nil
}

func (c *Client) Latest() (uint64, []byte, error) {
	resp, err := c.do(request{
		ID:      "1",
		Version: "2.0",
		Method:  "eth_getBlockByNumber",
		Params:  []any{"latest", false},
	})
	if err != nil {
		return 0, nil, fmt.Errorf("unable request hash: %w", err)
	}
	defer resp.Close()
	bresp := headerResp{}
	if err := json.NewDecoder(c.debug(resp)).Decode(&bresp); err != nil {
		return 0, nil, fmt.Errorf("unable to decode json into response: %w", err)
	}
	if err := check(bresp.Error); err != nil {
		return 0, nil, fmt.Errorf("eth_getBlockByNumber/latest: %w", err)
	}
	return uint64(bresp.Number), bresp.Hash, nil
}

func (c *Client) Hash(n uint64) ([]byte, error) {
	resp, err := c.do(request{
		ID:      "1",
		Version: "2.0",
		Method:  "eth_getBlockByNumber",
		Params:  []any{"0x" + strconv.FormatUint(n, 16), false},
	})
	if err != nil {
		return nil, fmt.Errorf("unable request hash: %w", err)
	}
	defer resp.Close()
	bresp := headerResp{}
	if err := json.NewDecoder(resp).Decode(&bresp); err != nil {
		return nil, fmt.Errorf("unable to decode json into response: %w", err)
	}
	if err := check(bresp.Error); err != nil {
		return nil, fmt.Errorf("eth_getBlockByNumber/hash: %w", err)
	}
	return bresp.Hash, nil
}

func (c *Client) LoadBlocks(f [][]byte, blocks []eth.Block) error {
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
		return fmt.Errorf("requesting txs: %w", err)
	}
	defer resp.Close()
	if err := json.NewDecoder(c.debug(resp)).Decode(&resps); err != nil {
		return fmt.Errorf("unable to decode json into response: %w", err)
	}
	for i := range resps {
		if err := check(resps[i].Error); err != nil {
			return fmt.Errorf("eth_getBlockByNumber/blocks: %w", err)
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
		n := "0x" + strconv.FormatUint(blocks[i].Num(), 16)
		reqs[i] = request{
			ID:      "1",
			Version: "2.0",
			Method:  "eth_getBlockByNumber",
			Params:  []any{n, false},
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
		if err := check(resps[i].Error); err != nil {
			return fmt.Errorf("eth_getBlockByNumber/headers: %w", err)
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
		n := "0x" + strconv.FormatUint(blocks[i].Num(), 16)
		reqs[i] = request{
			ID:      "1",
			Version: "2.0",
			Method:  "eth_getBlockReceipts",
			Params:  []any{n},
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
		if err := check(resps[i].Error); err != nil {
			return fmt.Errorf("eth_getBlockReceipts: %w", err)
		}
	}
	var blocksByNum = map[uint64]*eth.Block{}
	for i := range blocks {
		blocksByNum[blocks[i].Num()] = &blocks[i]
	}
	for i := range resps {
		b, ok := blocksByNum[uint64(resps[i].Result[0].BlockNum)]
		if !ok {
			return fmt.Errorf("block not found")
		}
		b.Header.Hash = resps[i].Result[0].BlockHash
		for j := range resps[i].Result {
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
}

type logResp struct {
	Error  `json:"error"`
	Result []logResult `json:"result"`
}

func (c *Client) logs(blocks []eth.Block) error {
	lf := struct {
		From    string     `json:"fromBlock"`
		To      string     `json:"toBlock"`
		Address []string   `json:"address"`
		Topics  [][]string `json:"topics"`
	}{
		From:    "0x" + strconv.FormatUint(blocks[0].Num(), 16),
		To:      "0x" + strconv.FormatUint(blocks[len(blocks)-1].Num(), 16),
		Address: c.filter.Address,
		Topics:  c.filter.Topics,
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
	if err := check(lresp.Error); err != nil {
		return fmt.Errorf("eth_getLogs: %w", err)
	}

	slices.SortFunc(lresp.Result, func(a, b logResult) int {
		return cmp.Compare(a.Log.Idx, b.Log.Idx)
	})
	var blocksByNum = map[uint64]*eth.Block{}
	for i := range blocks {
		blocksByNum[blocks[i].Num()] = &blocks[i]
	}

	type key struct {
		b uint64
		t uint64
	}
	var logsByTx = map[key][]logResult{}
	for i := range lresp.Result {
		k := key{
			b: uint64(lresp.Result[i].BlockNum),
			t: uint64(lresp.Result[i].TxIdx),
		}
		if logs, ok := logsByTx[k]; ok {
			logsByTx[k] = append(logs, lresp.Result[i])
			continue
		}
		logsByTx[k] = []logResult{lresp.Result[i]}
	}
	for k, logs := range logsByTx {
		b, ok := blocksByNum[k.b]
		if !ok {
			return fmt.Errorf("block not found")
		}
		tx := eth.Tx{}
		tx.Idx = eth.Uint64(k.t)
		tx.PrecompHash.Write(logs[0].TxHash)
		tx.Logs = make([]eth.Log, 0, len(logs))
		for i := range logs {
			tx.Logs = append(tx.Logs, *logs[i].Log)
		}
		b.Txs = append(b.Txs, tx)
	}
	return nil
}
