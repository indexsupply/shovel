package rlps

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/indexsupply/x/bint"
	"github.com/indexsupply/x/bloom"
	"github.com/indexsupply/x/e2pg"
	"github.com/indexsupply/x/eth"
	"github.com/indexsupply/x/freezer"
	"github.com/indexsupply/x/geth"
	"github.com/indexsupply/x/jrpc"
	"github.com/indexsupply/x/rlp"
)

func NewClient(chainID uint64, url string) *Client {
	return &Client{
		chainID: chainID,
		surl:    url,
		hc:      &http.Client{},
	}
}

type Client struct {
	chainID uint64
	surl    string
	hc      *http.Client
}

func (c *Client) ChainID() uint64 { return c.chainID }

var bufferPool = sync.Pool{New: func() any { return new(bytes.Buffer) }}

func (c *Client) LoadBlocks(filter [][]byte, bfs []geth.Buffer, blocks []eth.Block) error {
	// Use hash in the request url to avoid having the
	// rlps cdn serve a reorganized block.
	h, err := c.Hash(bfs[0].Number)
	if err != nil {
		return fmt.Errorf("unable to get hash for %d: %w", bfs[0].Number, err)
	}
	u, err := url.Parse(c.surl + "/blocks")
	if err != nil {
		return fmt.Errorf("unable to parse rpls server url")
	}
	q := u.Query()
	q.Add("n", strconv.FormatUint(bfs[0].Number, 10))
	q.Add("h", hex.EncodeToString(h))
	q.Add("limit", strconv.Itoa(len(blocks)))
	q.Add("filter", unparseFilter(filter))
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return fmt.Errorf("unable to make rlps request: %w", err)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("unable to execute rlps request: %w", err)
	}
	defer resp.Body.Close()
	enc := bufferPool.Get().(*bytes.Buffer)
	enc.Reset()
	defer bufferPool.Put(enc)

	_, err = io.Copy(enc, resp.Body)
	if err != nil {
		return fmt.Errorf("unable to read rlps response")
	}
	if (resp.StatusCode / 100) != 2 {
		return fmt.Errorf("rlps error: %s", enc.String())
	}
	for i, bitr := 0, rlp.Iter(enc.Bytes()); bitr.HasNext(); i++ {
		var (
			blockRLP    = rlp.Iter(bitr.Bytes())
			headerRLP   = blockRLP.Bytes()
			bodiesRLP   = blockRLP.Bytes()
			receiptsRLP = blockRLP.Bytes()
		)
		blocks[i].UnmarshalRLP(headerRLP)
		blocks[i].Txs.UnmarshalRLP(rlp.Bytes(bodiesRLP))
		blocks[i].Receipts.UnmarshalRLP(receiptsRLP)
	}
	return nil
}

func (c *Client) Latest() (uint64, []byte, error) {
	u, err := url.Parse(c.surl + "/latest")
	if err != nil {
		return 0, nil, fmt.Errorf("unable to parse rpls server url")
	}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return 0, nil, fmt.Errorf("unable to make rlps request")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("unable to execute rlps request: %w", err)
	}
	defer resp.Body.Close()
	enc, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, fmt.Errorf("unable to read rlps response")
	}
	if (resp.StatusCode / 100) != 2 {
		return 0, nil, fmt.Errorf("rlps error: %s", string(enc))
	}
	itr := rlp.Iter(enc)
	return bint.Uint64(itr.Bytes()), itr.Bytes(), nil
}

func (c *Client) Hash(n uint64) ([]byte, error) {
	u, err := url.Parse(c.surl + "/hash")
	if err != nil {
		return nil, fmt.Errorf("unable to parse rpls server url")
	}
	q := u.Query()
	q.Add("n", strconv.FormatUint(n, 10))
	u.RawQuery = q.Encode()
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("unable to make rlps request")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unable to execute rlps request: %w", err)
	}
	defer resp.Body.Close()
	enc, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read rlps response")
	}
	if (resp.StatusCode / 100) != 2 {
		return nil, fmt.Errorf("rlps error: %s", string(enc))
	}
	return enc, nil
}

func NewServer(fc freezer.FileCache, rc *jrpc.Client) *Server {
	s := &Server{fc: fc, rc: rc}
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/blocks", s.Blocks)
	s.mux.HandleFunc("/latest", s.Latest)
	s.mux.HandleFunc("/hash", s.Hash)
	s.mux.HandleFunc("/", s.Index)
	return s
}

type Server struct {
	fc freezer.FileCache
	rc *jrpc.Client

	mux *http.ServeMux
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

var zero = []byte{}

func (s *Server) get(filter [][]byte, n, limit uint64) ([]byte, error) {
	var bufs = make([]geth.Buffer, limit)
	for i := uint64(0); i < limit; i++ {
		bufs[i].Number = n + i
	}
	res := make([][]byte, limit)
	err := geth.Load(filter, bufs, s.fc, s.rc)
	if err != nil {
		return nil, fmt.Errorf("loading geth data: %w", err)
	}
	for i := 0; i < len(bufs); i++ {
		hrlp := bufs[i].Header()
		block := eth.Block{}
		block.UnmarshalRLP(hrlp)
		switch {
		case e2pg.Skip(filter, bloom.Filter(block.LogsBloom)):
			res[i] = rlp.List(rlp.Encode(hrlp, zero, zero))
		default:
			res[i] = rlp.List(rlp.Encode(
				hrlp,
				bufs[i].Bodies(),
				bufs[i].Receipts(),
			))
		}
	}
	return rlp.List(res...), nil
}

func (s *Server) Index(w http.ResponseWriter, r *http.Request) {
	num, hash, err := geth.Latest(s.rc)
	if err != nil {
		fmt.Fprintf(w, "error: %s\n", err)
	}
	fmt.Fprintf(w, "%d %x\n%s\n", bint.Uint64(num), hash, doc)
}

func (s *Server) Blocks(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.URL.Query().Get("n"))
	if err != nil {
		http.Error(w, "n must be an int", http.StatusBadRequest)
		return
	}
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil {
		http.Error(w, "limit must be an int", http.StatusBadRequest)
		return
	}
	filter, err := parseFilter(r.URL.Query().Get("filter"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rlpb, err := s.get(filter, uint64(n), uint64(limit))
	if err != nil {
		fmt.Printf("data load error: %s\n", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, err = w.Write(rlpb)
	if err != nil {
		fmt.Printf("write error: %s\n", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

func (s *Server) Latest(w http.ResponseWriter, r *http.Request) {
	num, hash, err := geth.Latest(s.rc)
	if err != nil {
		fmt.Printf("latest: %s\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	_, err = w.Write(rlp.List(rlp.Encode(num, hash)))
	if err != nil {
		fmt.Printf("write error: %s\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) Hash(w http.ResponseWriter, r *http.Request) {
	num, err := strconv.ParseUint(r.URL.Query().Get("n"), 10, 64)
	if err != nil {
		fmt.Printf("error: %s\n", err)
		http.Error(w, "n must be an int", http.StatusBadRequest)
		return
	}
	res, err := geth.Hash(num, s.fc, s.rc)
	if err != nil {
		fmt.Printf("error: %s\n", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = w.Write(res)
	if err != nil {
		fmt.Printf("write error: %s\n", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
	}
}

func unparseFilter(filter [][]byte) string {
	var s strings.Builder
	s.Grow(len(filter) * 32)
	for i := range filter {
		s.WriteString(hex.EncodeToString(filter[i]))
		if i != len(filter)-1 {
			s.WriteString(",")
		}
	}
	return s.String()
}

func parseFilter(filter string) ([][]byte, error) {
	if len(filter) == 0 {
		return nil, nil
	}
	var events [][]byte
	for _, s := range strings.Split(filter, ",") {
		b, err := hex.DecodeString(s)
		if err != nil {
			return nil, fmt.Errorf("filter must be hex encoded csv")
		}
		if len(b) != 32 {
			return nil, fmt.Errorf("filter item must be 32 bytes. got: %d", len(b))
		}
		events = append(events, b)
	}
	return events, nil
}

const doc = `

Latest

	Returns number and hash of latest block

	> https://rlps.indexsupply.net/latest
	< rlp([uint64, uint8[32]])

Hash

	Returns hash of block number n

	> GET https://rlps.indexsupply.net/hash?n=XXX
	< uint8[32]

Blocks

	Returns [n, n+limit) blocks starting at block number n

	Filter is a csv of hex encoded, 32 byte event signatures. If a filter value
	is in the header's bloom filter then the bodies and receipts are
	materialized. Otherwise the header is materialized but bodies
	and receipts are an empty RLP list.

	> GET https://rlps.indexsupply.net/blocks?n=XXX&limit=YYY&filter=ZZZ
	< rlp([[header, bodies, receipts], ..., [header, bodies, receipts]])
`
