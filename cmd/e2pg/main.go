package main

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"runtime/pprof"
	"strings"
	"time"

	"github.com/indexsupply/x/e2pg"
	"github.com/indexsupply/x/freezer"
	"github.com/indexsupply/x/integrations/erc1155"
	"github.com/indexsupply/x/integrations/erc721"
	"github.com/indexsupply/x/jrpc"
	"github.com/indexsupply/x/rlps"

	"github.com/jackc/pgx/v5/pgxpool"
)

func check(err error) {
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(1)
	}
}

func main() {
	var (
		ctx         = context.Background()
		freezerPath string
		pgURL       string
		rpcURL      string
		rlpsURL     string
		intgs       string
		listen      string
		workers     int
		batchSize   int
		useTx       bool
		reset       bool
		begin, end  int
		profile     string
		version     bool
	)

	flag.StringVar(&freezerPath, "f", "/storage/geth/geth/chaindata/ancient/chain/", "path to freezer files")
	flag.StringVar(&pgURL, "pg", "postgres:///e2pg", "postgres url")
	flag.StringVar(&rpcURL, "r", "http://zeus:8545", "address or socket for rpc server")
	flag.StringVar(&rlpsURL, "rlps", "", "use rlps for reading blockchain data")
	flag.StringVar(&intgs, "i", "all", "list of integrations")
	flag.StringVar(&listen, "l", ":8546", "dashboard server listen address")
	flag.IntVar(&workers, "w", 2, "number of concurrent workers")
	flag.IntVar(&batchSize, "b", 32, "batch size")
	flag.BoolVar(&useTx, "t", false, "use pg tx")
	flag.BoolVar(&reset, "reset", false, "drop public schame")
	flag.IntVar(&begin, "begin", -1, "starting block. -1 starts at latest")
	flag.IntVar(&end, "end", -1, "ending block. -1 never ends")
	flag.StringVar(&profile, "profile", "", "run profile after indexing")
	flag.BoolVar(&version, "version", false, "version")
	flag.Parse()

	if version {
		fmt.Printf("v%s-%s\n", Version, commit())
		os.Exit(0)
	}

	pgp, err := pgxpool.New(ctx, pgURL)
	check(err)

	if reset {
		h := pgp.Config().ConnConfig.Host
		if !(h == "localhost" || h == "/private/tmp" || h == "/var/run/postgresql") {
			fmt.Printf("unable to reset non-local db: %s\n", h)
			os.Exit(1)
		}
		_, err = pgp.Exec(ctx, "drop schema public cascade")
		check(err)
		_, err = pgp.Exec(ctx, "create schema public")
		check(err)
		for _, q := range strings.Split(e2pg.Schema, ";") {
			_, err = pgp.Exec(ctx, q)
			check(err)
		}
	}

	var (
		all = map[string]e2pg.Integration{
			"erc721":  erc721.Integration,
			"erc1155": erc1155.Integration,
		}
		running []e2pg.Integration
	)
	switch {
	case intgs == "all":
		for _, ig := range all {
			running = append(running, ig)
		}
	default:
		for _, name := range strings.Split(intgs, ",") {
			ig, ok := all[name]
			if !ok {
				check(fmt.Errorf("unable to find integration: %q", name))
			}
			running = append(running, ig)
		}
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

	var node e2pg.Node
	switch {
	case rlpsURL != "":
		node = rlps.NewClient(rlpsURL)
	default:
		node = e2pg.NewGeth(freezer.New(freezerPath), rc)
	}

	var (
		pbuf  bytes.Buffer
		drv   = e2pg.NewDriver(batchSize, workers, node, pgp, running...)
		snaps = make(chan e2pg.StatusSnapshot)
		dh    = newDashHandler(drv, snaps)
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/", dh.Index)
	mux.HandleFunc("/updates", dh.Updates)
	mux.HandleFunc("/debug/pprof/profile", func(w http.ResponseWriter, r *http.Request) {
		w.Write(pbuf.Bytes())
	})
	go http.ListenAndServe(listen, mux)

	gethNum, gethHash, err := node.Latest()
	check(err)
	fmt.Printf("node: %d %x\n", gethNum, gethHash)
	localNum, localHash, err := e2pg.Latest(pgp)
	check(err)
	fmt.Printf("e2pg: %d %x\n", localNum, localHash)

	switch {
	case begin == -1 && len(localHash) == 0:
		h, err := node.Hash(gethNum - 1)
		check(err)
		check(e2pg.Insert(pgp, gethNum-1, h))
	case begin != -1 && len(localHash) == 0:
		h, err := node.Hash(uint64(begin) - 1)
		check(err)
		check(e2pg.Insert(pgp, uint64(begin)-1, h))
	case begin != -1 && len(localHash) != 0:
		check(fmt.Errorf("-begin not available for initialized driver"))
	}

	if profile == "cpu" {
		check(pprof.StartCPUProfile(&pbuf))
	}
	t0 := time.Now()
	for {
		err = drv.Converge(useTx, uint64(end))
		if err == nil {
			go func() {
				snap := drv.Status()
				fmt.Printf("%s %s\n", snap.Num, snap.Hash)
				select {
				case snaps <- snap:
				default:
				}
			}()
			continue
		}
		if errors.Is(err, e2pg.ErrNothingNew) {
			time.Sleep(time.Second)
			continue
		}
		if errors.Is(err, e2pg.ErrDone) {
			fmt.Printf("elapsed: %s\n", time.Since(t0))
			break
		}
		if err != nil {
			fmt.Printf("error: %s\n", err)
		}
	}

	switch profile {
	case "cpu":
		pprof.StopCPUProfile()
		select {}
	case "heap":
		check(pprof.Lookup("heap").WriteTo(&pbuf, 0))
		select {}
	}
}

//Set using: go build -ldflags="-X main.Version=XXX"
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
