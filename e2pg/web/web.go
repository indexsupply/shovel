package web

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/indexsupply/x/e2pg"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Handler struct {
	pgp  *pgxpool.Pool
	mgr  *e2pg.Manager
	conf *e2pg.Config

	clientsMutex sync.Mutex
	clients      map[string]chan string
}

func New(mgr *e2pg.Manager, conf *e2pg.Config, pgp *pgxpool.Pool) *Handler {
	h := &Handler{
		pgp:     pgp,
		mgr:     mgr,
		conf:    conf,
		clients: make(map[string]chan string),
	}
	go func() {
		for {
			tid := mgr.Updates()
			for _, c := range h.clients {
				c <- tid
			}
		}
	}()
	return h
}

func read(file string) string {
	b, _ := os.ReadFile(fmt.Sprintf("./e2pg/web/%s.html", file))
	return string(b)
}

type IndexView struct {
	Igs            map[string][]e2pg.Integration
	TaskUpdates    []e2pg.TaskUpdate
	ChainsByTaskID map[string]e2pg.SourceConfig
}

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	var (
		ctx  = r.Context()
		view = IndexView{}
		err  error
	)
	view.TaskUpdates, err = e2pg.TaskUpdates(ctx, h.pgp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	view.Igs, err = h.conf.IntegrationsBySource(ctx, h.pgp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sources, err := h.conf.AllSourceConfigs(ctx, h.pgp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	view.ChainsByTaskID = map[string]e2pg.SourceConfig{}
	for i := range sources {
		k := fmt.Sprintf("%d-main", sources[i].ChainID)
		view.ChainsByTaskID[k] = sources[i]
	}

	tmpl, err := template.New("index").Parse(read("index"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	err = tmpl.Execute(w, view)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (h *Handler) Updates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	slog.InfoContext(r.Context(), "start sse", "c", r.RemoteAddr, "n", len(h.clients))
	c := make(chan string)
	h.clientsMutex.Lock()
	h.clients[r.RemoteAddr] = c
	h.clientsMutex.Unlock()
	defer func() {
		h.clientsMutex.Lock()
		delete(h.clients, r.RemoteAddr)
		h.clientsMutex.Unlock()
		close(c)
		slog.InfoContext(r.Context(), "stop sse", "c", r.RemoteAddr, "n", len(h.clients))
	}()

	for {
		var tid string
		select {
		case tid = <-c:
		case <-r.Context().Done(): // disconnect
			return
		}
		tu, err := e2pg.TaskUpdate1(r.Context(), h.pgp, tid)
		if err != nil {
			slog.ErrorContext(r.Context(), "json error", "e", err)
			return
		}
		sjson, err := json.Marshal(tu)
		if err != nil {
			slog.ErrorContext(r.Context(), "json error", "e", err)
			return
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
