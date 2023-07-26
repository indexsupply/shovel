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
	"github.com/indexsupply/x/pgmig"
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

		skipMigrate bool
		listen      string
		notx        bool
		profile     string
		version     bool
	)
	flag.StringVar(&cfile, "config", "", "task config file")
	flag.Uint64Var(&conf.ID, "id", 0, "task id")
	flag.Uint64Var(&conf.ChainID, "chain", 0, "task id")
	flag.StringVar(&conf.FreezerPath, "ef", "/storage/geth/geth/chaindata/ancient/chain/", "path to freezer files")
	flag.StringVar(&conf.PGURL, "pg", "", "postgres url")
	flag.StringVar(&conf.ETHURL, "e", "", "address or socket for rpc server")
	flag.Uint64Var(&conf.Concurrency, "c", 2, "number of concurrent workers")
	flag.Uint64Var(&conf.Batch, "b", 128, "batch size")
	flag.Uint64Var(&conf.Begin, "begin", 0, "starting block. 0 starts at latest")
	flag.Uint64Var(&conf.End, "end", 0, "ending block. 0 never ends")
	flag.StringVar(&intgs, "i", "", "list of integrations")

	flag.BoolVar(&skipMigrate, "skip-migrate", false, "do not run db migrations on startup")
	flag.StringVar(&listen, "l", ":8546", "dashboard server listen address")
	flag.BoolVar(&notx, "notx", false, "disable pg tx")
	flag.StringVar(&profile, "profile", "", "run profile after indexing")
	flag.BoolVar(&version, "version", false, "version")

	flag.Parse()

	if len(intgs) > 0 {
		for _, s := range strings.Split(intgs, ",") {
			conf.Integrations = append(conf.Integrations, s)
		}
	}

	if version {
		fmt.Printf("v%s-%s\n", Version, commit())
		os.Exit(0)
	}

	var confs []config.Config
	switch {
	case cfile != "" && !conf.Empty():
		fmt.Printf("unable to use config file and command line args\n")
		os.Exit(1)
	case cfile != "":
		f, err := os.Open(cfile)
		check(err)
		confs = []config.Config{}
		check(json.NewDecoder(f).Decode(&confs))
	case !conf.Empty():
		confs = []config.Config{conf}
	}

	if len(confs) == 0 {
		fmt.Println("Must specify at least 1 task configuration")
		os.Exit(1)
	}

	if !skipMigrate {
		migdb, err := pgxpool.New(ctx, config.Env(confs[0].PGURL))
		check(err)
		check(pgmig.Migrate(migdb, e2pg.Migrations))
		migdb.Close()
	}

	tasks, err := config.NewTasks(confs...)
	check(err)

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
			check(tasks[i].Run(snaps, notx))
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
