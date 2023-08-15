package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/indexsupply/x/rlp"
)

func check(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func main() {
	var (
		chain   uint64
		block   uint64
		txidx   int
		receipt bool
	)
	flag.Uint64Var(&chain, "c", 1, "chain id")
	flag.Uint64Var(&block, "b", 1, "block number")
	flag.IntVar(&txidx, "t", -1, "transaction index")
	flag.BoolVar(&receipt, "r", false, "receipt")
	flag.Parse()

	u, err := url.Parse(fmt.Sprintf("https://%d.rlps.indexsupply.net/blocks", chain))
	check(err)

	q := u.Query()
	q.Add("n", strconv.FormatUint(block, 10))
	q.Add("limit", "1")
	u.RawQuery = q.Encode()

	resp, err := http.Get(u.String())
	check(err)
	buf, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	check(err)
	if (resp.StatusCode / 100) != 2 {
		check(fmt.Errorf("rlps error: %s", string(buf)))
	}

	for i, bitr := 0, rlp.Iter(buf); bitr.HasNext(); i++ {
		var (
			blockRLP    = rlp.Iter(bitr.Bytes())
			headerRLP   = blockRLP.Bytes()
			bodiesRLP   = blockRLP.Bytes()
			receiptsRLP = blockRLP.Bytes()
		)
		switch {
		case txidx < 0:
			fmt.Printf("%x\n", headerRLP)
		case !receipt && txidx > 0:
			for j, it := 0, rlp.Iter(rlp.Bytes(bodiesRLP)); it.HasNext(); j++ {
				tx := it.Bytes()
				if j == txidx {
					fmt.Printf("%x\n", tx)
				}
			}
		case receipt && txidx < 0:
			fmt.Println("must specify -t for receipt")
		case receipt && txidx > 0:
			fmt.Printf("%x\n", receiptsRLP)
			for j, it := 0, rlp.Iter(bodiesRLP); it.HasNext(); j++ {
				r := it.Bytes()
				if j == txidx {
					fmt.Printf("%x\n", r)
				}
			}
		}
	}
}
