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
)

func New(chainID uint64, url string) *Client {
	return &Client{
		chainID: chainID,
		d:       strings.Contains(url, "debug"),
		hc:      &http.Client{},
		url:     url,
	}
}

type Client struct {
	d       bool
	chainID uint64
	url     string
	hc      *http.Client
}

func (c *Client) ChainID() uint64 { return c.chainID }

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

func (c *Client) LoadBlocks(f [][]byte, blocks []eth.Block) error {
	if err := c.blocks(blocks); err != nil {
		return fmt.Errorf("getting blocks: %w", err)
	}
	if err := c.logs(blocks); err != nil {
		return fmt.Errorf("getting logs: %w", err)
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
	for i, b := 0, new(eth.Block); i < len(lresp.Result); i++ {
		if uint64(lresp.Result[i].BlockNum) != b.Num() {
			for j := range blocks {
				if blocks[j].Num() == uint64(lresp.Result[i].BlockNum) {
					b = &blocks[j]
				}
			}
		}
		b.Txs[int(lresp.Result[i].TxIdx)].Logs = append(
			b.Txs[int(lresp.Result[i].TxIdx)].Logs,
			*lresp.Result[i].Log,
		)
	}
	return nil
}
