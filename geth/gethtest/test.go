package gethtest

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/golang/snappy"
	"github.com/indexsupply/x/bint"
	"github.com/indexsupply/x/geth/schema"
	"github.com/indexsupply/x/jrpc"
	"kr.dev/diff"
)

type Helper struct {
	FileCache *testFreezer
	Client    *jrpc.Client
	mockHTTP  *httptest.Server
	mrpc      *mockRPC
}

func New(tb testing.TB, fallback string) *Helper {
	tb.Helper()
	tdrc, err := jrpc.New(jrpc.WithHTTP(fallback))
	diff.Test(tb, tb.Fatalf, err, nil)
	mrpc := &mockRPC{tb: tb, tdrc: tdrc}
	mockHTTP := httptest.NewServer(mrpc)
	rc, err := jrpc.New(jrpc.WithHTTP(mockHTTP.URL))
	diff.Test(tb, tb.Fatalf, err, nil)
	return &Helper{
		FileCache: &testFreezer{tb: tb, tdrc: tdrc},
		Client:    rc,
		mockHTTP:  mockHTTP,
		mrpc:      mrpc,
	}
}

func (h *Helper) SetFreezerMax(n uint64) {
	h.FileCache.MaxNum = n
}

func (h *Helper) SetLatest(n uint64, hash []byte) {
	h.mrpc.LatestNum = bint.Encode(nil, n)
	h.mrpc.LatestHash = make([]byte, len(hash))
	copy(h.mrpc.LatestHash, hash)
}

func (h *Helper) Done() {
	h.mockHTTP.Close()
}

func jstr(s string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`"%s"`, s))
}

func jbytes(b []byte) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`"0x%s"`, hex.EncodeToString(b)))
}

func check(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func log(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Error(err)
	}
}

type testFreezer struct {
	tb     testing.TB
	tdrc   *jrpc.Client
	MaxNum uint64
}

func (f *testFreezer) Max(t string) (uint64, error) {
	return f.MaxNum, nil
}

func (f *testFreezer) File(string, uint64) (*os.File, int, int64, error) {
	panic("not implemented")
}

func (f *testFreezer) ReaderAt(t string, n uint64) (io.ReaderAt, int, int64, error) {
	b := get(f.tb, f.tdrc, t, n, nil)
	return bytes.NewReader(b), len(b), 0, nil
}

func modroot() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	for {
		fi, err := os.Stat(filepath.Join(dir, "go.mod"))
		if err == nil && !fi.IsDir() {
			return dir
		}
		d := filepath.Dir(dir)
		if d == dir {
			break
		}
		dir = d
	}
	return ""
}

func get(tb testing.TB, tdrc *jrpc.Client, t string, n uint64, h []byte) []byte {
	var cacheDir = filepath.Join(modroot(), "geth", "testdata")
	f := fmt.Sprintf("%d-%s", n, t)
	d, err := os.ReadFile(filepath.Join(cacheDir, f))
	if err == nil {
		return d
	}
	if !os.IsNotExist(err) {
		log(tb, err)
	}
	var (
		res = make([]jrpc.HexBytes, 1)
		tmp = []any{&res[0]}
	)
	err = tdrc.Request([]jrpc.Request{jrpc.Request{
		Version: "2.0",
		ID:      "1",
		Method:  "debug_dbAncient",
		Params: []json.RawMessage{
			jstr(t),
			json.RawMessage(strconv.Itoa(int(n))),
		},
	}}, tmp)
	if err == nil {
		// freezer reads are expected to be compressed.
		// data from debug_dbAncient returns uncompressed data.
		compressed := snappy.Encode(nil, res[0])
		log(tb, err)
		log(tb, os.MkdirAll(cacheDir, 0750))
		log(tb, os.WriteFile(filepath.Join(cacheDir, f), compressed, 0644))
		return compressed
	}
	err = tdrc.Request([]jrpc.Request{jrpc.Request{
		Version: "2.0",
		ID:      "1",
		Method:  "debug_dbGet",
		Params:  []json.RawMessage{jbytes(schema.Key(t, n, h))},
	}}, tmp)
	log(tb, err)
	compressed := snappy.Encode(nil, res[0])
	log(tb, err)
	log(tb, os.MkdirAll(cacheDir, 0750))
	log(tb, os.WriteFile(filepath.Join(cacheDir, f), compressed, 0644))
	return compressed
}

func mustDecompress(tb testing.TB, src []byte) []byte {
	res, err := snappy.Decode(nil, src)
	check(tb, err)
	return res
}

type mockRPC struct {
	tb   testing.TB
	tdrc *jrpc.Client

	LatestNum, LatestHash []byte
}

func (m *mockRPC) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	req := []jrpc.Request{}
	err := json.NewDecoder(r.Body).Decode(&req)
	defer r.Body.Close()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	resp := make([]jrpc.Response, len(req))
	for i := range req {
		var k jrpc.HexBytes
		err := json.Unmarshal(req[i].Params[0], &k)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		table, n, h := schema.ParseKey(k)
		switch {
		case string(k) == "LastBlock":
			resp[i].Result = jbytes(m.LatestHash)
		case k[0] == 'H':
			resp[i].Result = jbytes(m.LatestNum)
		default:
			b := get(m.tb, m.tdrc, table, n, h)
			b = mustDecompress(m.tb, b)
			resp[i].Result = jbytes(b)
		}
	}
	if err = json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
}
