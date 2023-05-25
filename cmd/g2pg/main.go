package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"fmt"
	_ "net/http/pprof"
	"os"
	"strings"
	"time"

	"github.com/indexsupply/x/g2pg"
	"github.com/indexsupply/x/gethdb"
	"github.com/indexsupply/x/integrations/nftxfr"
	"github.com/indexsupply/x/jrpc"

	"github.com/jackc/pgx/v5/pgxpool"
)

func check(err error) {
	if err != nil {
		fmt.Printf("%s\n", err)
		os.Exit(1)
	}
}

//go:embed schema.sql
var schema string

func main() {
	var (
		ctx         = context.Background()
		freezerPath string
		pgURL       string
		rpcURL      string
		intgs       string
		listen      string
		workers     int
		batchSize   int
		useTx       bool
		reset       bool
		begin, end  int
	)

	flag.StringVar(&freezerPath, "f", "/storage/geth/geth/chaindata/ancient/chain/", "path to freezer files")
	flag.StringVar(&pgURL, "pg", "postgres:///g2pg", "postgres url")
	flag.StringVar(&rpcURL, "r", "http://zeus:8545", "address or socket for rpc server")
	flag.StringVar(&intgs, "i", "all", "list of integrations")
	flag.StringVar(&listen, "l", ":8546", "dashboard server listen address")
	flag.IntVar(&workers, "w", 1<<6, "number of concurrent workers")
	flag.IntVar(&batchSize, "b", 1<<11, "batch size")
	flag.BoolVar(&useTx, "t", false, "use pg tx")
	flag.BoolVar(&reset, "reset", false, "drop public schame")
	flag.IntVar(&begin, "begin", -1, "starting block. -1 starts at latest")
	flag.IntVar(&end, "end", -1, "ending block. -1 never ends")
	flag.Parse()

	pgp, err := pgxpool.New(ctx, pgURL)
	check(err)

	if reset {
		h := pgp.Config().ConnConfig.Host
		if !(h == "localhost" || h == "/private/tmp") {
			fmt.Printf("unable to reset non-local db: %s\n", h)
			os.Exit(1)
		}
		_, err = pgp.Exec(ctx, "drop schema public cascade")
		check(err)
		_, err = pgp.Exec(ctx, "create schema public")
		check(err)
		for _, q := range strings.Split(schema, ";") {
			_, err = pgp.Exec(ctx, q)
			check(err)
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

	var (
		all = map[string]g2pg.Integration{
			"nft": nftxfr.Integration,
		}
		running []g2pg.Integration
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

	var (
		snaps = make(chan g2pg.StatusSnapshot)
		drv   = g2pg.NewDriver(batchSize, workers, running...)
		g     = gethdb.New(freezerPath, rc)
	)

	go serveDashboard(listen, snaps, drv)

	gethNum, gethHash, err := g.Latest()
	check(err)
	fmt.Printf("geth: %d %x\n", gethNum, gethHash[:4])
	localNum, localHash, err := drv.Latest(pgp)
	check(err)
	fmt.Printf("g2pg: %d %x\n", localNum, localHash[:4])

	switch {
	case begin == -1 && localHash != [32]byte{}:
		break
	case begin == -1 && localHash == [32]byte{}:
		h, err := g.Hash(gethNum - 1)
		check(err)
		check(drv.Insert(pgp, gethNum-1, h))
	case begin != -1 && localHash == [32]byte{}:
		h, err := g.Hash(uint64(begin) - 1)
		check(err)
		check(drv.Insert(pgp, uint64(begin)-1, h))
	case begin != -1 && localHash != [32]byte{}:
		check(fmt.Errorf("-begin not available for initialized driver"))
	}

	for {
		err = drv.Converge(g, pgp, useTx, uint64(end))
		if errors.Is(err, g2pg.ErrNothingNew) {
			time.Sleep(time.Second)
			continue
		}
		go func() {
			snap := drv.Status()
			fmt.Printf("%s %s\n", snap.Num, snap.Hash)
			select {
			case snaps <- snap:
			default:
			}
		}()
		check(err)
	}
}
