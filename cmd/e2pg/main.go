package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"runtime/pprof"
	"strings"

	"github.com/indexsupply/x/e2pg"
	"github.com/indexsupply/x/e2pg/config"
	"golang.org/x/sync/errgroup"

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
		ctx   = context.Background()
		cfile string

		conf  config.Config
		intgs string

		listen  string
		usetx   bool
		reset   bool
		profile string
		version bool
	)
	flag.StringVar(&cfile, "config", "", "task config file")
	flag.StringVar(&conf.FreezerPath, "ef", "/storage/geth/geth/chaindata/ancient/chain/", "path to freezer files")
	flag.StringVar(&conf.PGURL, "pg", "postgres:///e2pg", "postgres url")
	flag.StringVar(&conf.ETHURL, "e", "http://zeus:8545", "address or socket for rpc server")
	flag.Uint64Var(&conf.Concurrency, "c", 2, "number of concurrent workers")
	flag.Uint64Var(&conf.Batch, "b", 32, "batch size")
	flag.StringVar(&intgs, "i", "all", "list of integrations")
	flag.Uint64Var(&conf.Begin, "begin", 0, "starting block. 0 starts at latest")
	flag.Uint64Var(&conf.End, "end", 0, "ending block. -1 never ends")

	flag.StringVar(&listen, "l", ":8546", "dashboard server listen address")
	flag.BoolVar(&usetx, "t", false, "use pg tx")
	flag.BoolVar(&reset, "reset", false, "drop public schame")
	flag.StringVar(&profile, "profile", "", "run profile after indexing")
	flag.BoolVar(&version, "version", false, "version")
	flag.Parse()

	if version {
		fmt.Printf("v%s-%s\n", Version, commit())
		os.Exit(0)
	}

	if reset {
		pgp, err := pgxpool.New(ctx, conf.PGURL)
		check(err)
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
		pgp.Close()
	}

	var tasks []*e2pg.Task
	switch {
	case cfile != "" && !conf.Empty():
		fmt.Printf("unable to use config file and command line args")
		os.Exit(1)
	case cfile != "":
		f, err := os.Open(cfile)
		check(err)
		confs := []config.Config{}
		check(json.NewDecoder(f).Decode(&confs))
		tasks, err = config.NewTasks(confs...)
		check(err)
	case !conf.Empty():
		var err error
		tasks, err = config.NewTasks(conf)
		check(err)
	}

	var (
		pbuf  bytes.Buffer
		snaps = make(chan e2pg.StatusSnapshot)
		dh    = newDashHandler(tasks, snaps)
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/", dh.Index)
	mux.HandleFunc("/updates", dh.Updates)
	mux.HandleFunc("/debug/pprof/profile", func(w http.ResponseWriter, r *http.Request) {
		w.Write(pbuf.Bytes())
	})
	go http.ListenAndServe(listen, mux)

	if profile == "cpu" {
		check(pprof.StartCPUProfile(&pbuf))
	}
	var eg errgroup.Group
	for i := range tasks {
		i := i
		eg.Go(func() error {
			check(tasks[i].Setup())
			check(tasks[i].Run(snaps, usetx))
			return nil
		})
	}
	eg.Wait()
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
