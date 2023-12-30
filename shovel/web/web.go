package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/indexsupply/x/eth"
	"github.com/indexsupply/x/shovel"
	"github.com/indexsupply/x/shovel/config"
	"github.com/indexsupply/x/shovel/glf"
	"github.com/indexsupply/x/wstrings"

	"filippo.io/age"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kr/session"
)

var (
	//go:embed index.html
	indexHTML string

	//go:embed login.html
	loginHTML string

	//go:embed diag.html
	diagHTML string

	//go:embed add-source.html
	addSourceHTML string

	//go:embed add-integration.html
	addIntegrationHTML string
)

var htmlpages = map[string]string{
	"index":           indexHTML,
	"diag":            diagHTML,
	"login":           loginHTML,
	"add-source":      addSourceHTML,
	"add-integration": addIntegrationHTML,
}

type Handler struct {
	local bool
	pgp   *pgxpool.Pool
	mgr   *shovel.Manager
	conf  *config.Root

	clientsMutex sync.Mutex
	clients      map[string]chan []byte

	templates map[string]*template.Template

	sess     session.Config
	password []byte
}

func New(mgr *shovel.Manager, conf *config.Root, pgp *pgxpool.Pool) *Handler {
	h := &Handler{
		pgp:       pgp,
		mgr:       mgr,
		conf:      conf,
		clients:   make(map[string]chan []byte),
		templates: make(map[string]*template.Template),
	}
	cookieID, err := age.GenerateX25519Identity()
	if err != nil {
		panic(err)
	}
	h.sess.Keys = append(h.sess.Keys, cookieID)
	h.password = []byte(conf.Dashboard.RootPassword)
	if len(h.password) == 0 {
		b := make([]byte, 8)
		rand.Read(b)
		h.password = make([]byte, hex.EncodedLen(len(b)))
		hex.Encode(h.password, b)
	}
	return h
}

func (h *Handler) Authn(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.conf.Dashboard.DisableAuthn {
			next(w, r)
			return
		}
		if !h.conf.Dashboard.EnableLoopbackAuthn && isLoopback(r) {
			next(w, r)
			return
		}
		err := session.Get(r, &struct{}{}, &h.sess)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if h.conf.Dashboard.RootPassword == "" {
		slog.InfoContext(r.Context(), "random-temp-password",
			"password", string(h.password),
		)
	}
	switch r.Method {
	case "GET":
		tmpl, err := h.template("login")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := tmpl.Execute(w, nil); err != nil {
			slog.ErrorContext(r.Context(), "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case "POST":
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		supplied := []byte(r.FormValue("password"))
		if subtle.ConstantTimeCompare(supplied, h.password) != 1 {
			http.Error(w, "invalid password", http.StatusUnauthorized)
			return
		}
		if isLoopback(r) {
			c := session.DefaultCookie
			c.Secure = false
			h.sess.Cookie = &c
		}
		session.Set(w, &struct{}{}, &h.sess)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	default:
		http.Error(w, "must be post or get", http.StatusMethodNotAllowed)
		return
	}
}

func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	return net.ParseIP(host).IsLoopback()
}

type DiagResult struct {
	Name    string
	Message string
	Latency time.Duration
}

func (h *Handler) Diag(w http.ResponseWriter, r *http.Request) {
	var (
		res []DiagResult
		ctx = r.Context()
	)
	run := func(name string, f func() string) {
		t := time.Now()
		m := f()
		res = append(res, DiagResult{
			Name:    fmt.Sprintf("%-10s", strings.ToLower(name)),
			Message: fmt.Sprintf("%20s", m),
			Latency: time.Since(t),
		})
	}
	run("Postgres", func() string {
		var x uint64
		err := h.pgp.QueryRow(ctx, "select num from shovel.task_updates limit 1").Scan(&x)
		if err != nil {
			return err.Error()
		}
		return "ok"
	})
	scs, err := h.conf.AllSources(ctx, h.pgp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, sc := range scs {
		src := shovel.NewSource(sc, glf.Filter{})
		run(sc.Name, func() string {
			n, h, err := src.Latest()
			if err != nil {
				return fmt.Sprintf("latest: %s\n", err.Error())
			}
			b := []eth.Block{eth.Block{Header: eth.Header{Number: eth.Uint64(n)}}}
			if err := src.LoadBlocks(nil, b); err != nil {
				return fmt.Sprintf("loading blocks: %s\n", err.Error())
			}
			if !bytes.Equal(b[0].Hash(), h) {
				return fmt.Sprintf("block mismatch: %.4x != %.4x\n",
					b[0].Hash(),
					h,
				)
			}
			return fmt.Sprintf("%d %.4x", n, h)
		})
	}
	tmpl, err := h.template("diag")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, res); err != nil {
		slog.ErrorContext(r.Context(), "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
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
		ig  = config.Integration{}
	)
	defer r.Body.Close()
	err = json.NewDecoder(r.Body).Decode(&ig)
	if err != nil {
		slog.ErrorContext(ctx, "decoding integration", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	testConfig := config.Root{Integrations: []config.Integration{ig}}
	if err := config.CheckUserInput(testConfig); err != nil {
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
	srcs, err := h.conf.AllSources(ctx, h.pgp)
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
	TaskUpdates  map[string]shovel.TaskUpdate
	IGUpdates    map[string][]shovel.IGUpdate
	Sources      []config.Source
	ShowBackfill bool
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

	view.Sources, err = h.conf.AllSources(ctx, h.pgp)
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
