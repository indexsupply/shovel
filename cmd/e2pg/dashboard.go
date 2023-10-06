package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"text/template"

	"github.com/indexsupply/x/e2pg"
)

type dashHandler struct {
	tasks        []*e2pg.Task
	clientsMutex sync.Mutex
	clients      map[string]chan e2pg.StatusSnapshot
}

func newDashHandler(tasks []*e2pg.Task, snaps <-chan e2pg.StatusSnapshot) *dashHandler {
	dh := &dashHandler{
		tasks:   tasks,
		clients: make(map[string]chan e2pg.StatusSnapshot),
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

	slog.InfoContext(r.Context(), "start sse", "c", r.RemoteAddr, "n", len(dh.clients))
	c := make(chan e2pg.StatusSnapshot)
	dh.clientsMutex.Lock()
	dh.clients[r.RemoteAddr] = c
	dh.clientsMutex.Unlock()
	defer func() {
		dh.clientsMutex.Lock()
		delete(dh.clients, r.RemoteAddr)
		dh.clientsMutex.Unlock()
		close(c)
		slog.InfoContext(r.Context(), "stop sse", "c", r.RemoteAddr, "n", len(dh.clients))
	}()

	for {
		var snap e2pg.StatusSnapshot
		select {
		case snap = <-c:
		case <-r.Context().Done(): // disconnect
			return
		}
		sjson, err := json.Marshal(snap)
		if err != nil {
			slog.ErrorContext(r.Context(), "json error", "e", err)
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
				<meta charset="utf-8"><title>E2PG</title>
				<style>
					body {
						font-family: 'SF Mono', ui-monospace, monospace;
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
				<h1>E2PG</h1>
				<div id="tasks">
					<table id="tasks-status">
						<thead>
							<tr>
								<td>Task</td>
								<td>Chain</td>
								<td>Block</td>
								<td>Hash</td>
								<td>Blocks</td>
								<td>Events</td>
								<td>Latency</td>
								<td>Error</td>
							</tr>
						</thead>
						<tbody>
							{{ range $id, $status := . -}}
							{{ $name := $status.Name -}}
							<tr>
								<td id="{{ printf "%s-name" $name}}">{{ $name }}</td>
								<td id="{{ printf "%s-id" $name}}">{{ $id }}</td>
								<td id="{{ printf "%s-num" $name}}">{{ $status.Num }}</td>
								<td id="{{ printf "%s-hash" $name}}">{{ $status.Hash }}</td>
								<td id="{{ printf "%s-block_count" $name}}">{{ $status.BlockCount }}</td>
								<td id="{{ printf "%s-event_count" $name}}">{{ $status.EventCount }}</td>
								<td id="{{ printf "%s-total_latency_p50" $name}}">{{ $status.TotalLatencyP50 }}</td>
								<td id="{{ printf "%s-error" $name}}">{{ $status.Error }}</td>
							</tr>
							{{ end -}}
						</tbody>
					</table>
				</div>
				<script>
					function update(snap, field) {
						const elm = document.getElementById(snap.name + "-" + field);
						elm.textContent = snap[field];
						elm.classList.add("highlight");
						setTimeout(function() {
							elm.classList.remove("highlight")
						}, 1000);
					};
					var updates = new EventSource("/updates");
					updates.onmessage = function(event) {
						const snap = JSON.parse(event.data);
						update(snap, "num");
						update(snap, "hash");
						update(snap, "block_count");
						update(snap, "event_count");
						update(snap, "total_latency_p50");
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
	snaps := make(map[uint64]e2pg.StatusSnapshot)
	for _, task := range dh.tasks {
		snaps[task.ID] = task.Status()
	}
	err = tmpl.Execute(w, snaps)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
