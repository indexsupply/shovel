// JSON RPC client
package jrpc2

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	bresp := blockResp{Block: &eth.Block{}}
	if err := json.NewDecoder(c.debug(resp)).Decode(&bresp); err != nil {
		return 0, nil, fmt.Errorf("unable to decode json into response: %w", err)
	}
	if bresp.Error.Code != 0 {
		return 0, nil, fmt.Errorf("rpc error: %s %d %s",
			"eth_getBlockByNumber-latest",
			bresp.Error.Code,
			bresp.Error.Message,
		)
	}
	return bresp.Num(), bresp.Hash(), nil
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
	bresp := blockResp{Block: &eth.Block{}}
	if err := json.NewDecoder(resp).Decode(&bresp); err != nil {
		return nil, fmt.Errorf("unable to decode json into response: %w", err)
	}
	if bresp.Error.Code != 0 {
		return nil, fmt.Errorf("rpc error: %s %d %s",
			"eth_getBlockByNumber-hash",
			bresp.Error.Code,
			bresp.Error.Message,
		)
	}
	return bresp.Hash(), nil
}

func (c *Client) LoadBlocks(f [][]byte, blocks []eth.Block) error {
	if err := c.blocks(blocks); err != nil {
		return fmt.Errorf("getting blocks: %w", err)
	}
	for i := range blocks {
		blocks[i].Receipts = make(eth.Receipts, len(blocks[i].Txs))
	}
	if err := c.logs(blocks); err != nil {
		return fmt.Errorf("getting logs: %w", err)
	}
	return nil
}

type blockResp struct {
	*eth.Block `json:"result"`
	Error      struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) blocks(blocks []eth.Block) error {
	if !c.filter.UseBlocks {
		slog.Debug("jrpc2: skipping blocks")
		return nil
	}
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
			Params:  []any{n, c.filter.UseTxs},
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
		if resps[i].Error.Code != 0 {
			return fmt.Errorf("rpc error: %s %d %s",
				"eth_getBlockByNumber",
				resps[i].Error.Code,
				resps[i].Error.Message,
			)
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
	Removed   bool       `json:"removed"`
}

type receiptResp struct {
	Result []receiptResult `json:"result"`
	Error  struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) receipts(blocks []eth.Block) error {
	if !c.filter.UseReceipts {
		slog.Debug("jrpc2: skipping receipts")
		return nil
	}
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
		if resps[i].Error.Code == 0 {
			continue
		}
		return fmt.Errorf("rpc error: %s %d %s",
			"eth_getBlockByNumber",
			resps[i].Error.Code,
			resps[i].Error.Message,
		)
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
		b.Txs = make(eth.Txs, len(resps[i].Result))
		b.Receipts = make(eth.Receipts, len(resps[i].Result))
		for j := range resps[i].Result {
			b.Txs[j].PrecompHash = resps[i].Result[j].TxHash
			b.Txs[j].Type = resps[i].Result[j].TxType
			b.Txs[j].From = resps[i].Result[j].TxFrom
			b.Txs[j].To = resps[i].Result[j].TxTo
			b.Receipts[j].Status = resps[i].Result[j].Status
			b.Receipts[j].GasUsed = resps[i].Result[j].GasUsed
			b.Receipts[j].Logs.Copy(resps[i].Result[j].Logs)
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
	LogIdx    eth.Uint64 `json:"logIndex"`
	Removed   bool       `json:"removed"`
}

type logResp struct {
	Result []logResult `json:"result"`
	Error  struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) logs(blocks []eth.Block) error {
	if !c.filter.UseLogs {
		slog.Debug("jrpc2: skipping logs")
		return nil
	}
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
	if lresp.Error.Code != 0 {
		return fmt.Errorf("rpc error: %s %d %s",
			"eth_getLogs",
			lresp.Error.Code,
			lresp.Error.Message,
		)
	}
	slices.SortFunc(lresp.Result, func(a, b logResult) int {
		return cmp.Compare(a.LogIdx, b.LogIdx)
	})
	var blocksByNum = map[uint64]*eth.Block{}
	for i := range blocks {
		blocksByNum[blocks[i].Num()] = &blocks[i]
	}
	for i := 0; i < len(lresp.Result); i++ {
		b, ok := blocksByNum[uint64(lresp.Result[i].BlockNum)]
		if !ok {
			return fmt.Errorf("block not found")
		}
		b.Header.Hash = lresp.Result[i].BlockHash
		var (
			tx *eth.Tx
			re *eth.Receipt
		)
		ti := int(lresp.Result[i].TxIdx)
		b.Txs, tx = get(b.Txs, ti)
		b.Receipts, re = get(b.Receipts, ti)
		tx.PrecompHash = lresp.Result[i].TxHash
		re.Logs = append(re.Logs, *lresp.Result[i].Log)
	}
	return nil
}

// returns pointer to x[i]. extends x if i >= len(x)
func get[X ~[]E, E any](x X, i int) (X, *E) {
	switch {
	case i < 0:
		panic("cannot be negative")
	case i < len(x):
		return x, &x[i]
	case i >= len(x):
		n := max(1, i+1-len(x))
		x = append(x, make(X, n)...)
		return x, &x[i]
	default:
		panic("default")
	}
}
