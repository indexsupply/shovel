package jrpc2

import (
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/indexsupply/x/eth"
	"kr.dev/diff"
)

var (
	//go:embed testdata/block-18000000.json
	block18000000JSON string
	//go:embed testdata/logs-18000000.json
	logs18000000JSON string

	//go:embed testdata/block-1000001.json
	block1000001JSON string
	//go:embed testdata/logs-1000001.json
	logs1000001JSON string
)

func TestNoLogs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		diff.Test(t, t.Fatalf, nil, err)
		switch {
		case strings.Contains(string(body), "eth_getBlockByNumber"):
			_, err := w.Write([]byte(block1000001JSON))
			diff.Test(t, t.Fatalf, nil, err)
		case strings.Contains(string(body), "eth_getLogs"):
			_, err := w.Write([]byte(logs1000001JSON))
			diff.Test(t, t.Fatalf, nil, err)
		}
	}))
	defer ts.Close()

	blocks := []eth.Block{eth.Block{Header: eth.Header{Number: 1000001}}}
	c := New(0, ts.URL)
	err := c.LoadBlocks(nil, nil, blocks)
	diff.Test(t, t.Errorf, nil, err)

	b := blocks[0]
	diff.Test(t, t.Errorf, len(b.Txs), 1)
	diff.Test(t, t.Errorf, len(b.Receipts), 1)

	r := blocks[0].Receipts[0]
	diff.Test(t, t.Errorf, len(r.Logs), 0)
}

func TestLatest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		diff.Test(t, t.Fatalf, nil, err)
		switch {
		case strings.Contains(string(body), "eth_getBlockByNumber"):
			_, err := w.Write([]byte(block18000000JSON))
			diff.Test(t, t.Fatalf, nil, err)
		case strings.Contains(string(body), "eth_getLogs"):
			_, err := w.Write([]byte(logs18000000JSON))
			diff.Test(t, t.Fatalf, nil, err)
		}
	}))
	defer ts.Close()

	blocks := []eth.Block{eth.Block{Header: eth.Header{Number: 18000000}}}
	c := New(0, ts.URL)
	err := c.LoadBlocks(nil, nil, blocks)
	diff.Test(t, t.Errorf, nil, err)

	b := blocks[0]
	diff.Test(t, t.Errorf, b.Num(), uint64(18000000))
	diff.Test(t, t.Errorf, fmt.Sprintf("%.4x", b.Parent), "198723e0")
	diff.Test(t, t.Errorf, fmt.Sprintf("%.4x", b.Hash()), "95b198e1")
	diff.Test(t, t.Errorf, b.Time, eth.Uint64(1693066895))
	diff.Test(t, t.Errorf, fmt.Sprintf("%.4x", b.LogsBloom), "53f146f2")
	diff.Test(t, t.Errorf, len(b.Txs), 94)
	diff.Test(t, t.Errorf, len(b.Receipts), 94)

	tx0 := blocks[0].Txs[0]
	diff.Test(t, t.Errorf, fmt.Sprintf("%.4x", tx0.Hash()), "16e19967")
	diff.Test(t, t.Errorf, fmt.Sprintf("%.4x", tx0.To), "fd14567e")
	diff.Test(t, t.Errorf, fmt.Sprintf("%s", tx0.Value.Dec()), "0")

	signer, err := tx0.Signer()
	diff.Test(t, t.Errorf, nil, err)
	diff.Test(t, t.Errorf, fmt.Sprintf("%.4x", signer), "16d5783a")

	tx3 := blocks[0].Txs[3]
	diff.Test(t, t.Errorf, fmt.Sprintf("%s", tx3.Value.Dec()), "69970000000000014")

	r := blocks[0].Receipts[0]
	diff.Test(t, t.Errorf, len(r.Logs), 1)

	l := blocks[0].Receipts[0].Logs[0]
	diff.Test(t, t.Errorf, fmt.Sprintf("%.4x", l.Address), "fd14567e")
	diff.Test(t, t.Errorf, fmt.Sprintf("%.4x", l.Topics[0]), "b8b9c39a")
	diff.Test(t, t.Errorf, fmt.Sprintf("%x", l.Data), "01e14e6ce75f248c88ee1187bcf6c75f8aea18fbd3d927fe2d63947fcd8cb18c641569e8ee18f93c861576fe0c882e5c61a310ae8e400be6629561160d2a901f0619e35040579fa202bc3f84077a72266b2a4e744baa92b433497bc23d6aeda4")
}
