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
	"github.com/indexsupply/x/geth"
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
	return bresp.Hash(), nil
}

func (c *Client) LoadBlocks(f [][]byte, _ []geth.Buffer, blocks []eth.Block) error {
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
	slices.SortFunc(lresp.Result, func(a, b logResult) int {
		return cmp.Compare(a.LogIdx, b.LogIdx)
	})
	for i, b := 0, new(eth.Block); i < len(lresp.Result); i++ {
		if uint64(lresp.Result[i].BlockNum) != b.Num() {
			for j := range blocks {
				if blocks[j].Num() == uint64(lresp.Result[i].BlockNum) {
					b = &blocks[j]
				}
			}
		}
		b.Receipts[int(lresp.Result[i].TxIdx)].Logs = append(
			b.Receipts[int(lresp.Result[i].TxIdx)].Logs,
			*lresp.Result[i].Log,
		)
	}
	return nil
}
