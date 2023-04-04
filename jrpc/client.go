package jrpc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    []byte `json:"data"`
}

type Response struct {
	Version string `json:"jsonrpc"`
	ID      string `json:"id"`
	Result  string `json:"result"`
	Error   Error  `json:"error"`
}

type Request struct {
	Version string            `json:"jsonrpc"`
	ID      string            `json:"id"`
	Method  string            `json:"method"`
	Params  []json.RawMessage `json:"params"`
}

type Client struct {
	url string
	hc  *http.Client
}

type Option func(c *Client)

func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) {
		c.hc = hc
	}
}

func WithURL(url string) Option {
	return func(c *Client) {
		c.url = url
	}
}

func New(opts ...Option) *Client {
	c := &Client{}
	for _, opt := range opts {
		opt(c)
	}
	if c.hc == nil {
		tr := &http.Transport{
			MaxIdleConns:    10,
			IdleConnTimeout: 10 * time.Second,
		}
		c.hc = &http.Client{Transport: tr}
	}
	return c
}

func (c Client) Request(req Request) (Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return Response{}, fmt.Errorf("unable to marshal request into json: %w", err)
	}
	hreq, err := http.NewRequest("POST", c.url+"/", bytes.NewReader(body))
	if err != nil {
		return Response{}, fmt.Errorf("unable to new request: %w", err)
	}
	hreq.Header.Add("content-type", "application/json")
	hresp, err := c.hc.Do(hreq)
	if err != nil {
		return Response{}, fmt.Errorf("unable to do http request: %w", err)
	}
	resp := Response{}
	err = json.NewDecoder(hresp.Body).Decode(&resp)
	defer hresp.Body.Close()
	if err != nil {
		return Response{}, fmt.Errorf("unable to decode json into response: %w", err)
	}
	if resp.Error.Code != 0 {
		const es = "rpc error code=%d message=%q"
		return Response{}, fmt.Errorf(es, resp.Error.Code, resp.Error.Message)
	}
	return resp, nil
}

func (c Client) EthCall(contract [20]byte, data []byte) ([]byte, error) {
	latest := json.RawMessage(`"latest"`)
	params, err := json.Marshal(struct {
		To   string `json:"to"`
		Data string `json:"data"`
	}{fmt.Sprintf("0x%x", contract), fmt.Sprintf("0x%x", data)})
	if err != nil {
		return nil, fmt.Errorf("marshaling eth_call params: %w", err)
	}
	resp, err := c.Request(Request{
		Version: "2.0",
		ID:      "1",
		Method:  "eth_call",
		Params:  []json.RawMessage{params, latest},
	})
	if err != nil {
		return nil, fmt.Errorf("making eth_call: %w", err)
	}
	res, err := hex.DecodeString(strings.TrimPrefix(resp.Result, "0x"))
	if err != nil {
		return nil, fmt.Errorf("decoding hex result: %w", err)
	}
	return res, nil
}
