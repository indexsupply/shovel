package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/indexsupply/x/freezer"
	"github.com/indexsupply/x/jrpc"
	"github.com/indexsupply/x/rlps"
)

func check(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func main() {
	var (
		err         error
		listen      string
		freezerPath string
		rpcURL      string
	)
	flag.StringVar(&freezerPath, "f", "/storage/geth/geth/chaindata/ancient/chain/", "path to freezer files")
	flag.StringVar(&listen, "l", ":8546", "listen for connections on addr:port")
	flag.StringVar(&rpcURL, "r", "http://zeus:8545", "address or socket for rpc server")
	flag.Parse()

	var rc *jrpc.Client
	switch {
	case strings.HasPrefix(rpcURL, "http"):
		rc, err = jrpc.New(jrpc.WithHTTP(rpcURL))
		check(err)
	default:
		const defaultSocketPath = "/storage/geth/geth.ipc"
		rc, err = jrpc.New(jrpc.WithSocket(defaultSocketPath))
		check(err)
	}
	h := &http.Server{
		Addr:           listen,
		Handler:        rlps.NewServer(freezer.New(freezerPath), rc),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	check(h.ListenAndServe())
}
