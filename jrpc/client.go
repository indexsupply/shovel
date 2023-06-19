package jrpc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    []byte `json:"data"`
}

type Response struct {
	Version string          `json:"jsonrpc"`
	ID      string          `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   Error           `json:"error"`
}

type Request struct {
	Version string            `json:"jsonrpc"`
	ID      string            `json:"id"`
	Method  string            `json:"method"`
	Params  []json.RawMessage `json:"params"`
}

type Client struct {
	url string
	ipc net.Conn
	hc  *http.Client
}

type Option func(c *Client) error

func WithHTTP(url string) Option {
	return func(c *Client) error {
		tr := &http.Transport{
			MaxIdleConns:    10,
			IdleConnTimeout: 10 * time.Second,
		}
		c.hc = &http.Client{Transport: tr}
		c.url = url
		return nil
	}
}

func WithSocket(path string) Option {
	return func(c *Client) (err error) {
		c.ipc, err = net.Dial("unix", path)
		return err
	}
}

func New(opts ...Option) (*Client, error) {
	c := &Client{}
	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, fmt.Errorf("jrpc new client: %w", err)
		}
	}
	if c.hc == nil && c.ipc == nil || c.hc != nil && c.ipc != nil {
		return nil, fmt.Errorf("jrpc must set ipc XOR http")
	}
	return c, nil
}

func (c Client) Request(req []Request, dest []any) error {
	var resp = []Response{}
	switch {
	case c.ipc != nil:
		err := json.NewEncoder(c.ipc).Encode(req)
		if err != nil {
			return fmt.Errorf("writing ipc request: %w", err)
		}
		err = json.NewDecoder(c.ipc).Decode(&resp)
		if err != nil {
			return fmt.Errorf("reading ipc response: %w", err)
		}
	case c.hc != nil:
		body, err := json.Marshal(req)
		if err != nil {
			return fmt.Errorf("unable to marshal request into json: %w", err)
		}
		hreq, err := http.NewRequest("POST", c.url+"/", bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("unable to new request: %w", err)
		}
		hreq.Header.Add("content-type", "application/json")
		hresp, err := c.hc.Do(hreq)
		if err != nil {
			return fmt.Errorf("unable to do http request: %w", err)
		}
		err = json.NewDecoder(hresp.Body).Decode(&resp)
		defer hresp.Body.Close()
		if err != nil {
			return fmt.Errorf("unable to decode json into response: %w", err)
		}
	default:
		return fmt.Errorf("must set ipc or http")
	}
	for i := range resp {
		if resp[i].Error.Code != 0 {
			const es = "rpc error code=%d message=%q"
			return fmt.Errorf(es, resp[i].Error.Code, resp[i].Error.Message)
		}
		if err := json.Unmarshal(resp[i].Result, dest[i]); err != nil {
			return fmt.Errorf("unable to decode json into dest type: %w", err)
		}
	}
	return nil
}

func (c Client) request1(req Request, dest any) error {
	return c.Request([]Request{req}, []any{dest})
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
	var hexData string
	err = c.request1(Request{
		Version: "2.0",
		ID:      "1",
		Method:  "eth_call",
		Params:  []json.RawMessage{params, latest},
	}, &hexData)
	if err != nil {
		return nil, fmt.Errorf("making eth_call: %w", err)
	}
	res, err := hex.DecodeString(strings.TrimPrefix(hexData, "0x"))
	if err != nil {
		return nil, fmt.Errorf("decoding hex result: %w", err)
	}
	return res, nil
}

func jbytes(b []byte) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`"0x%s"`, hex.EncodeToString(b)))
}

func jnum(n uint64) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`"0x%s"`, strconv.FormatUint(n, 16)))
}

func jstr(s string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`"%s"`, s))
}

func jbool(b bool) json.RawMessage {
	if b {
		return json.RawMessage("true")
	}
	return json.RawMessage("false")
}

type HexBytes []byte

func (b HexBytes) MarshalJSON() ([]byte, error) {
	return json.Marshal(hex.EncodeToString(b))
}

func (b *HexBytes) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}
	if len(s) < 2 || s[0] != '0' || (s[0] == 'x' || s[1] == 'X') {
		return fmt.Errorf("expected 0x prefix on hex string")
	}
	*b, err = hex.DecodeString(s[2:])
	return err
}

func (c Client) Get(dst []HexBytes, keys [][]byte) error {
	requests := make([]Request, len(keys))
	for i := range keys {
		requests[i] = Request{
			Version: "2.0",
			ID:      "1",
			Method:  "debug_dbGet",
			Params:  []json.RawMessage{jbytes(keys[i])},
		}
	}
	// https://go.dev/doc/faq#convert_slice_of_interface
	tmp := make([]any, len(dst))
	for i := range dst {
		tmp[i] = &dst[i]
	}
	return c.Request(requests, tmp)
}

func (c Client) Get1(key []byte) ([]byte, error) {
	res := make([]HexBytes, 1)
	err := c.Get(res, [][]byte{key})
	if err != nil {
		return nil, err
	}
	return res[0], nil
}
