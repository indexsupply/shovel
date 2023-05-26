package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"text/template"

	"github.com/indexsupply/x/g2pg"
)

type dashHandler struct {
	drv          *g2pg.Driver
	clientsMutex sync.Mutex
	clients      map[string]chan g2pg.StatusSnapshot
}

func newDashHandler(drv *g2pg.Driver, snaps <-chan g2pg.StatusSnapshot) *dashHandler {
	dh := &dashHandler{
		drv:     drv,
		clients: make(map[string]chan g2pg.StatusSnapshot),
	}
	go func() {
		for {
			snap := <-snaps
			for _, c := range dh.clients {
				c <- snap
			}
		}
	}()
	return dh
}

func (dh *dashHandler) Updates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	fmt.Printf("(%d) opening %s\n", len(dh.clients), r.RemoteAddr)
	c := make(chan g2pg.StatusSnapshot)
	dh.clientsMutex.Lock()
	dh.clients[r.RemoteAddr] = c
	dh.clientsMutex.Unlock()
	defer func() {
		dh.clientsMutex.Lock()
		delete(dh.clients, r.RemoteAddr)
		dh.clientsMutex.Unlock()
		close(c)
		fmt.Printf("(%d) closing %s\n", len(dh.clients), r.RemoteAddr)
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
}

func (dh *dashHandler) Index(w http.ResponseWriter, r *http.Request) {
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
	tmpl, err := template.New("dashboard").Parse(dashboardHTML)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = tmpl.Execute(w, dh.drv.Status())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
