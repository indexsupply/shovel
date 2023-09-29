package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
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
		verbose     bool
		version     bool
	)
	flag.StringVar(&freezerPath, "f", "/storage/geth/geth/chaindata/ancient/chain/", "path to freezer files")
	flag.StringVar(&listen, "l", ":8546", "listen for connections on addr:port")
	flag.StringVar(&rpcURL, "r", "http://zeus:8545", "address or socket for rpc server")
	flag.BoolVar(&verbose, "v", false, "verbose stdout logging")
	flag.BoolVar(&version, "version", false, "print version and exit")
	flag.Parse()

	if version {
		fmt.Printf("v%s-%s\n", Version, commit())
		os.Exit(0)
	}

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
		Handler:        log(verbose, rlps.NewServer(freezer.New(freezerPath), rc)),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	check(h.ListenAndServe())
}

func log(v bool, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t0 := time.Now()
		h.ServeHTTP(w, r) // serve the original request
		if v {
			fmt.Printf("%s\t%s\n", r.URL.Path, time.Since(t0))
		}
	})
}

// Set using: go build -ldflags="-X main.Version=XXX"
var Version string

func commit() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "ernobuildinfo"
	}
	var (
		revision = "missing"
		modified bool
	)
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value[:8]
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if !modified {
		return revision
	}
	return revision + "-modified"
}
