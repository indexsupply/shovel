package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	npprof "net/http/pprof"
	"os"
	"runtime/debug"
	"runtime/pprof"
	"time"

	"github.com/indexsupply/x/e2pg"
	"github.com/indexsupply/x/e2pg/web"
	"github.com/indexsupply/x/pgmig"
	"github.com/indexsupply/x/wctx"
	"github.com/indexsupply/x/wos"
	"github.com/indexsupply/x/wslog"

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

		skipMigrate bool
		listen      string
		notx        bool
		profile     string
		version     bool
	)
	flag.StringVar(&cfile, "config", "", "task config file")
	flag.BoolVar(&skipMigrate, "skip-migrate", false, "do not run db migrations on startup")
	flag.StringVar(&listen, "l", ":8546", "dashboard server listen address")
	flag.BoolVar(&notx, "notx", false, "disable pg tx")
	flag.StringVar(&profile, "profile", "", "run profile after indexing")
	flag.BoolVar(&version, "version", false, "version")

	flag.Parse()

	lh := wslog.New(os.Stdout, nil)
	lh.RegisterContext(func(ctx context.Context) (string, any) {
		id := wctx.ChainID(ctx)
		if id < 1 {
			return "", nil
		}
		return "chain", fmt.Sprintf("%.5d", id)
	})
	lh.RegisterContext(func(ctx context.Context) (string, any) {
		id := wctx.TaskID(ctx)
		if id == "" {
			return "", nil
		}
		return "task", id
	})
	slog.SetDefault(slog.New(lh.WithAttrs([]slog.Attr{
		slog.Int("p", os.Getpid()),
		slog.String("v", Commit),
	})))

	if version {
		fmt.Printf("v%s-%s\n", Version, Commit)
		os.Exit(0)
	}

	var (
		conf  e2pg.Config
		pgurl string
	)
	switch {
	case cfile == "":
		pgurl = os.Getenv("DATABASE_URL")
	case cfile != "":
		f, err := os.Open(cfile)
		check(err)
		check(json.NewDecoder(f).Decode(&conf))
		pgurl = wos.Getenv(conf.PGURL)
	}

	if !skipMigrate {
		migdb, err := pgxpool.New(ctx, pgurl)
		check(err)
		check(pgmig.Migrate(migdb, e2pg.Migrations))
		migdb.Close()
	}

	pg, err := pgxpool.New(ctx, pgurl)
	check(err)
	var (
		pbuf bytes.Buffer
		mgr  = e2pg.NewManager(pg, conf)
		wh   = web.New(mgr, &conf, pg)
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/", wh.Index)
	mux.HandleFunc("/task-updates", wh.Updates)
	mux.HandleFunc("/debug/pprof/", npprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", npprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", npprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", npprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", npprof.Trace)
	mux.HandleFunc("/debug/pprof/capture", func(w http.ResponseWriter, r *http.Request) {
		w.Write(pbuf.Bytes())
	})
	go http.ListenAndServe(listen, log(true, mux))

	if profile == "cpu" {
		check(pprof.StartCPUProfile(&pbuf))
	}

	go mgr.Run()

	switch profile {
	case "cpu":
		pprof.StopCPUProfile()
	case "heap":
		check(pprof.Lookup("heap").WriteTo(&pbuf, 0))
	}
	select {}
}

func log(v bool, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t0 := time.Now()
		h.ServeHTTP(w, r) // serve the original request
		if v {
			slog.Info("", "e", time.Since(t0), "u", r.URL.Path)
		}
	})
}

// Set using: go build -ldflags="-X main.Version=XXX"
var (
	Version string
	Commit  = func() string {
		bi, ok := debug.ReadBuildInfo()
		if !ok {
			return "ernobuildinfo"
		}
		var (
			revision = ""
			modified bool
		)
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				revision = s.Value[:4]
			case "vcs.modified":
				modified = s.Value == "true"
			}
		}
		if !modified {
			return revision
		}
		return revision + "-"
	}()
)
