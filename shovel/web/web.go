package web

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/indexsupply/x/shovel"
	"github.com/indexsupply/x/wstrings"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	//go:embed index.html
	indexHTML string

	//go:embed add-source.html
	addSourceHTML string

	//go:embed add-integration.html
	addIntegrationHTML string
)

var htmlpages = map[string]string{
	"index":           indexHTML,
	"add-source":      addSourceHTML,
	"add-integration": addIntegrationHTML,
}

type Handler struct {
	local bool
	pgp   *pgxpool.Pool
	mgr   *shovel.Manager
	conf  *shovel.Config

	clientsMutex sync.Mutex
	clients      map[string]chan []byte

	templates map[string]*template.Template
}

func New(mgr *shovel.Manager, conf *shovel.Config, pgp *pgxpool.Pool) *Handler {
	return &Handler{
		pgp:       pgp,
		mgr:       mgr,
		conf:      conf,
		clients:   make(map[string]chan []byte),
		templates: make(map[string]*template.Template),
	}
}

func (h *Handler) PushUpdates() error {
	ctx := context.Background()
	for ; ; h.mgr.Updates() {
		tus, err := shovel.TaskUpdates(ctx, h.pgp)
		if err != nil {
			return fmt.Errorf("querying task updates: %w", err)
		}
		for _, update := range tus {
			j, err := json.Marshal(update)
			if err != nil {
				return fmt.Errorf("marshaling task update: %w", err)
			}
			for _, c := range h.clients {
				c <- j
			}
		}
		ius, err := shovel.IGUpdates(ctx, h.pgp)
		if err != nil {
			return fmt.Errorf("querying ig updates: %w", err)
		}
		for _, update := range ius {
			j, err := json.Marshal(update)
			if err != nil {
				return fmt.Errorf("marshaling ig update: %w", err)
			}
			for _, c := range h.clients {
				c <- j
			}
		}
	}
}

func (h *Handler) template(name string) (*template.Template, error) {
	if h.local {
		b, err := os.ReadFile(fmt.Sprintf("./shovel/web/%s.html", name))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		return template.New(name).Parse(string(b))
	}
	t, ok := h.templates[name]
	if ok {
		return t, nil
	}
	html, ok := htmlpages[name]
	if !ok {
		return nil, fmt.Errorf("unable to find html for %s", name)
	}
	t, err := template.New(name).Parse(html)
	if err != nil {
		return nil, fmt.Errorf("parsing template %s: %w", name, err)
	}
	h.templates[name] = t
	return t, nil
}

func (h *Handler) SaveIntegration(w http.ResponseWriter, r *http.Request) {
	var (
		err error
		ctx = r.Context()
		ig  = shovel.Integration{}
	)
	defer r.Body.Close()
	err = json.NewDecoder(r.Body).Decode(&ig)
	if err != nil {
		slog.ErrorContext(ctx, "decoding integration", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	testConfig := shovel.Config{Integrations: []shovel.Integration{ig}}
	if err := testConfig.CheckUserInput(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cj, err := json.Marshal(ig)
	if err != nil {
		slog.ErrorContext(ctx, "encoding integration", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	const q = `insert into shovel.integrations(name, conf) values ($1, $2)`
	_, err = h.pgp.Exec(ctx, q, ig.Name, cj)
	if err != nil {
		slog.ErrorContext(ctx, "inserting integration", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.mgr.Restart(); err != nil {
		slog.ErrorContext(ctx, "saving integration", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode("ok")
}

type AddIntegrationView struct {
	Sources json.RawMessage
}

func (h *Handler) AddIntegration(w http.ResponseWriter, r *http.Request) {
	var (
		err  error
		ctx  = r.Context()
		view = AddIntegrationView{}
	)
	srcs, err := h.conf.AllSourceConfigs(ctx, h.pgp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	view.Sources, err = json.Marshal(srcs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tmpl, err := h.template("add-integration")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, view); err != nil {
		slog.ErrorContext(ctx, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

type IndexView struct {
	TaskUpdates   map[string]shovel.TaskUpdate
	IGUpdates     map[string][]shovel.IGUpdate
	SourceConfigs []shovel.SourceConfig
	ShowBackfill  bool
}

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	var (
		ctx  = r.Context()
		view = IndexView{}
	)
	tus, err := shovel.TaskUpdates(ctx, h.pgp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ius, err := shovel.IGUpdates(ctx, h.pgp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	view.IGUpdates = make(map[string][]shovel.IGUpdate)
	for _, iu := range ius {
		view.IGUpdates[iu.TaskID()] = append(
			view.IGUpdates[iu.TaskID()],
			iu,
		)
	}
	view.TaskUpdates = make(map[string]shovel.TaskUpdate)
	for _, tu := range tus {
		view.TaskUpdates[tu.DOMID] = tu
	}

	for _, tu := range view.TaskUpdates {
		if tu.Backfill {
			view.ShowBackfill = true
			break
		}
	}

	view.SourceConfigs, err = h.conf.AllSourceConfigs(ctx, h.pgp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	t, err := h.template("index")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := t.Execute(w, view); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (h *Handler) Updates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	slog.InfoContext(r.Context(), "start sse", "c", r.RemoteAddr, "n", len(h.clients))
	c := make(chan []byte)
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
		select {
		case j := <-c:
			fmt.Fprintf(w, "data: %s\n\n", j)
			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
				return
			}
			flusher.Flush()
		case <-r.Context().Done(): // disconnect
			return
		}
	}
}

func (h *Handler) AddSource(w http.ResponseWriter, r *http.Request) {
	t, err := h.template("add-source")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := t.Execute(w, nil); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (h *Handler) SaveSource(w http.ResponseWriter, r *http.Request) {
	var (
		ctx = r.Context()
		err = r.ParseForm()
	)
	chainID, err := strconv.Atoi(r.FormValue("chainID"))
	if err != nil {
		slog.ErrorContext(ctx, "parsing chain id", err)
		return
	}
	name := r.FormValue("name")
	if len(name) == 0 {
		slog.ErrorContext(ctx, "parsing chain name", err)
		return
	}
	if err := wstrings.Safe(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	url := r.FormValue("ethURL")
	if len(url) == 0 {
		slog.ErrorContext(ctx, "parsing chain eth url", err)
		return
	}
	const q = `
		insert into shovel.sources(chain_id, name, url)
		values ($1, $2, $3)
	`
	_, err = h.pgp.Exec(ctx, q, chainID, name, url)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		slog.ErrorContext(ctx, "inserting task", err)
		return
	}
	if err := h.mgr.Restart(); err != nil {
		slog.ErrorContext(ctx, "saving source", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}