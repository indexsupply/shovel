package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/indexsupply/x/g2pg"
	"github.com/indexsupply/x/gethdb"
	"github.com/indexsupply/x/integrations/nftxfr"
	"github.com/indexsupply/x/jrpc"
	"github.com/indexsupply/x/txlocker"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	var (
		freezerPath string
		pgURL       string
		rpcURL      string
		intgs       string
		listen      string
		workers     uint64
		batchSize   uint64
		useTx       bool
	)

	flag.StringVar(&freezerPath, "f", "/storage/geth/geth/chaindata/ancient/chain/", "path to freezer files")
	flag.StringVar(&pgURL, "pg", "postgres:///g2pg", "postgres url")
	flag.StringVar(&rpcURL, "r", "http://zeus:8545", "address or socket for rpc server")
	flag.StringVar(&intgs, "i", "all", "list of integrations")
	flag.StringVar(&listen, "l", ":8546", "dashboard server listen address")
	flag.Uint64Var(&workers, "w", 1<<6, "number of concurrent workers")
	flag.Uint64Var(&batchSize, "b", 1<<11, "batch size")
	flag.BoolVar(&useTx, "t", false, "use pg tx")
	flag.Parse()

	var (
		ctx = context.Background()
		rc  *jrpc.Client
		err error
	)
	switch {
	case strings.HasPrefix(rpcURL, "http"):
		rc, err = jrpc.New(jrpc.WithHTTP(rpcURL))
		check(err)
	default:
		const defaultSocketPath = "/storage/geth/geth.ipc"
		rc, err = jrpc.New(jrpc.WithSocket(defaultSocketPath))
		check(err)
	}
	g := gethdb.New(freezerPath, rc)
	pg, err := pgxpool.New(ctx, pgURL)
	check(err)

	var (
		all = map[string]g2pg.Integration{
			"nft": nftxfr.Integration,
		}
		running []g2pg.Integration
		snaps   = make(chan g2pg.StatusSnapshot)
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
	drv := g2pg.NewDriver(batchSize, workers, running...)
	go serveDashboard(listen, snaps, drv)
	index(ctx, g, pg, useTx, drv, snaps)
}

func index(ctx context.Context, g gethdb.Handle, pgpool *pgxpool.Pool, useTx bool, drv *g2pg.Driver, snapshots chan<- g2pg.StatusSnapshot) {
	for reorgCount := 0; ; {
		var (
			err      error
			pg       g2pg.PG = pgpool
			pgTx     pgx.Tx
			commit   = func() {}
			rollback = func() {}
		)
		if useTx {
			pgTx, err = pgpool.Begin(ctx)
			check(err)
			pg = txlocker.NewTx(pgTx)
			commit = func() { check(pgTx.Commit(ctx)) }
			rollback = func() { check(pgTx.Rollback(ctx)) }
		}
		err = drv.IndexBatch(ctx, g, pg)
		switch {
		case err == nil:
			reorgCount = 0
			commit()
			snap := drv.Status()
			fmt.Printf("%s %s\n", snap.Num, snap.Hash)
			select {
			case snapshots <- snap:
			default:
			}
		case errors.Is(err, g2pg.ErrNothingNew):
			reorgCount = 0
			rollback()
			time.Sleep(time.Second)
		case errors.Is(err, g2pg.ErrReorg):
			commit()
			reorgCount++
			if reorgCount > 10 {
				fmt.Printf("reog limit exceeded")
				os.Exit(1)
			}
		default:
			fmt.Printf("unhandled error: %s\n", err)
			os.Exit(1)
		}
	}
}

func serveDashboard(listen string, snaps <-chan g2pg.StatusSnapshot, drv *g2pg.Driver) {
	const dashboardHTML = `
		<!DOCTYPE html>
		<html>
			<head>
				<meta charset="utf-8"><title>g2pg</title>
				<style>
					body {
						font-family: "Courier New", Courier, monospace;
						max-width: 800px;
						margin: 0 auto;
					}
					.highlight {
						background-color: lightyellow;
						transition: background-color 0.5s ease-out;
					}
					table {
						border-collapse: collapse;
						width: 100%;
						max-width: 800px;
						margin: 0 auto;
						background-color: white;
					}
					td, th {
						border: 1px solid darkgray;
						padding: 0.5em;
						text-align: left;
					}
				</style>
			</head>
			<body>
				<h1>g2pg</h1>
				<div id="driver">
					<table id="driver-status">
						<thead>
							<tr>
								<td>Block</td>
								<td>Hash</td>
								<td>Blocks</td>
								<td>Events</td>
								<td>Latency</td>
								<td>Error</td>
							</tr>
						</thead>
						<tbody>
							<tr>
								<td id="num">{{ .Num }}</td>
								<td id="hash">{{ .Hash }}</td>
								<td id="block-count">{{ .BlockCount }}</td>
								<td id="event-count">{{ .EventCount }}</td>
								<td id="latency">{{ .TotalLatencyP50 }}</td>
								<td>{{ .Error }}</td>
							</tr>
						</tbody>
					</table>
				</div>
				<script>
					var updates = new EventSource("/updates");
					updates.onmessage = function(event) {
						const snap = JSON.parse(event.data);
						const numDiv = document.getElementById('num');
						numDiv.textContent = snap.num;
						numDiv.classList.add("highlight");
						setTimeout(function() {
							numDiv.classList.remove("highlight")
						}, 1000);

						const hashDiv = document.getElementById('hash');
						hashDiv.textContent = snap.hash;
						hashDiv.classList.add("highlight")
						setTimeout(function() {
							hashDiv.classList.remove("highlight")
						}, 1000);

						const blockCountDiv = document.getElementById('block-count');
						blockCountDiv.textContent = snap.block_count;
						blockCountDiv.classList.add("highlight")
						setTimeout(function() {
							blockCountDiv.classList.remove("highlight")
						}, 1000);

						const eventCountDiv = document.getElementById('event-count');
						eventCountDiv.textContent = snap.event_count;
						eventCountDiv.classList.add("highlight")
						setTimeout(function() {
							eventCountDiv.classList.remove("highlight")
						}, 1000);

						const latDiv = document.getElementById('latency');
						latDiv.textContent = snap.total_latency_p50;
					};
				</script>
			</body>
		</html>
	`
	var (
		clientsMutex sync.Mutex
		clients      = make(map[string]chan g2pg.StatusSnapshot)
	)
	go func() {
		for {
			snap := <-snaps
			for _, c := range clients {
				c <- snap
			}
		}
	}()
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		tmpl, err := template.New("dashboard").Parse(dashboardHTML)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		err = tmpl.Execute(w, drv.Status())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
	http.HandleFunc("/updates", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		fmt.Printf("(%d) opening %s\n", len(clients), r.RemoteAddr)
		c := make(chan g2pg.StatusSnapshot)
		clientsMutex.Lock()
		clients[r.RemoteAddr] = c
		clientsMutex.Unlock()
		defer func() {
			clientsMutex.Lock()
			delete(clients, r.RemoteAddr)
			clientsMutex.Unlock()
			close(c)
			fmt.Printf("(%d) closing %s\n", len(clients), r.RemoteAddr)
		}()

		for {
			var snap g2pg.StatusSnapshot
			select {
			case snap = <-c:
			case <-r.Context().Done():
				return
			}
			sjson, err := json.Marshal(snap)
			if err != nil {
				fmt.Printf("error: %v\n", err)
			}
			fmt.Fprintf(w, "data: %s\n\n", sjson)
			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
				return
			}
			flusher.Flush()
		}
	})
	http.ListenAndServe(listen, nil)
}
