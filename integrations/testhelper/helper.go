// Easily test g2pg integrations
//
package testhelper

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"blake.io/pqx/pqxtest"
	"github.com/indexsupply/x/bloom"
	"github.com/indexsupply/x/g2pg"
	"github.com/indexsupply/x/isxhash"
	"github.com/indexsupply/x/jrpc"
	"github.com/indexsupply/x/rlp"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/valyala/fastjson"
	"kr.dev/diff"
)

func jnum(n uint64) json.RawMessage {
	return json.RawMessage(`"0x` + strconv.FormatUint(n, 16) + `"`)
}

func jstr(s string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`"%s"`, s))
}

func check(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// I'm not a huge fan of using this relative path
// but it might 'just work ™️''.
// Typically the testdata directory would exist
// in the same directory as the package being tested,
// but that seems wasteful given that there might
// be overlap across integrations.
const cacheDir = "../testdata"

func get(t testing.TB, rc *jrpc.Client, kind string, num uint64) []byte {
	f := fmt.Sprintf("%d-%s", num, kind)
	d, err := os.ReadFile(filepath.Join(cacheDir, f))
	if err == nil {
		return d
	}
	if !os.IsNotExist(err) {
		check(t, err)
	}
	jp := new(fastjson.Parser)
	res, err := rc.Do(jp, nil, []jrpc.Request{jrpc.Request{
		Version: "2.0",
		ID:      "1",
		Method:  "debug_dbAncient",
		Params: []json.RawMessage{
			jstr(kind),
			json.RawMessage(strconv.Itoa(int(num))),
		},
	}})
	check(t, err)
	decoded := decode(t, res[0])
	check(t, os.MkdirAll(cacheDir, 0750))
	check(t, os.WriteFile(filepath.Join(cacheDir, f), decoded, 0644))
	return decoded

}

func decode(t testing.TB, input []byte) []byte {
	var p fastjson.Parser
	v, err := p.ParseBytes(input)
	check(t, err)
	s, err := v.StringBytes()
	check(t, err)
	b, err := hex.DecodeString(string(s[2:]))
	check(t, err)
	return b
}

type testGeth struct {
	tb         testing.TB
	rc         *jrpc.Client
	latestNum  uint64
	latestHash []byte
}

func (g *testGeth) Latest() (uint64, []byte, error) {
	return g.latestNum, g.latestHash, nil
}

func (g *testGeth) Hash(n uint64) ([]byte, error) {
	return isxhash.Keccak(get(g.tb, g.rc, "headers", n)), nil
}

func (g *testGeth) LoadBlocks(sf g2pg.SkipFunc, blocks []g2pg.Block) error {
	// TODO(r): I would like to remove this testing implentation
	// and rely entirely on g2pg's G implementation. To do that,
	// we will need to layout a freezer index based on the test data
	// that we download in the get function.
	for i := range blocks {
		rlpd := get(g.tb, g.rc, "headers", blocks[i].Number)
		blocks[i].Header.Unmarshal(rlpd)
		blocks[i].Hash = isxhash.Keccak(rlpd)

		if sf(bloom.Filter(blocks[i].Header.LogsBloom)) {
			continue
		}

		blocks[i].Transactions.Reset()
		rlpd = get(g.tb, g.rc, "bodies", blocks[i].Number)
		for j, it := 0, rlp.Iter(rlp.Bytes(rlpd)); it.HasNext(); j++ {
			blocks[i].Transactions.Insert(j, it.Bytes())
		}

		blocks[i].Receipts.Reset()
		rlpd = get(g.tb, g.rc, "receipts", blocks[i].Number)
		for j, it := 0, rlp.Iter(rlpd); it.HasNext(); j++ {
			blocks[i].Receipts.Insert(j, it.Bytes())
		}
	}
	return nil
}

func testpg(t testing.TB) *pgxpool.Pool {
	pqxtest.CreateDB(t, g2pg.Schema)
	pg, err := pgxpool.New(context.Background(), pqxtest.DSNForTest(t))
	diff.Test(t, t.Fatalf, nil, err)
	return pg
}

func testrc(t testing.TB) *jrpc.Client {
	rc, err := jrpc.New(jrpc.WithHTTP("http://zeus:8545"))
	diff.Test(t, t.Errorf, nil, err)
	return rc
}

type H struct {
	tb testing.TB
	DB *pgxpool.Pool
	rc *jrpc.Client
}

// jrpc.Client is required when testdata isn't
// available in the integrations/testdata directory.
func New(tb testing.TB) *H {
	return &H{
		tb: tb,
		DB: testpg(tb),
		rc: testrc(tb),
	}
}

// Reset the driver table. Call this in-between test cases
func (th *H) Reset() {
	_, err := th.DB.Exec(context.Background(), "truncate table driver")
	check(th.tb, err)
}

// Process will download the header,bodies, and receipts data
// if it doesn't exist in: integrations/testdata
// In the case that it needs to fetch the data, an RPC
// client will be used. The RPC endpoint needs to support
// the debug_dbAncient method.
func (th *H) Process(ig g2pg.Integration, n uint64) {
	var (
		geth   = &testGeth{tb: th.tb, rc: th.rc}
		driver = g2pg.NewDriver(1, 1, geth, th.DB, ig)
	)
	cur, err := geth.Hash(n)
	prev, err := geth.Hash(n - 1)
	geth.latestNum = n
	geth.latestHash = cur
	check(th.tb, driver.Insert(n-1, prev))

	check(th.tb, err)
	check(th.tb, driver.Converge(true, n))
}
