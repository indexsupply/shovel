package web

import (
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

	"github.com/indexsupply/shovel/jrpc2"
	"github.com/indexsupply/shovel/shovel"
	"github.com/indexsupply/shovel/shovel/config"
	"github.com/indexsupply/shovel/wstrings"

	"filippo.io/age"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kr/session"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	pgp  *pgxpool.Pool
	mgr  *shovel.Manager
	conf *config.Root
	rs   *shovel.RepairService

	clientsMutex sync.Mutex
	clients      map[string]chan []byte

	templates map[string]*template.Template

	sess     session.Config
	password []byte

	// global rate limit for diag requests
	diagLastReqMut sync.Mutex
	diagLastReq    time.Time
}

func New(mgr *shovel.Manager, conf *config.Root, pgp *pgxpool.Pool, rs *shovel.RepairService) *Handler {
	h := &Handler{
		pgp:       pgp,
		mgr:       mgr,
		conf:      conf,
		rs:        rs,
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

func (h *Handler) HandleRepairRequest(w http.ResponseWriter, r *http.Request) {
	h.rs.HandleRepairRequest(w, r)
}

func (h *Handler) HandleRepairStatus(w http.ResponseWriter, r *http.Request) {
	h.rs.HandleRepairStatus(w, r)
}

func (h *Handler) HandleListRepairs(w http.ResponseWriter, r *http.Request) {
	h.rs.HandleListRepairs(w, r)
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
		tmpl, err := h.template(isLoopback(r), "login")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := tmpl.Execute(w, nil); err != nil {
			slog.ErrorContext(r.Context(), "template", "error", err)
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
	Source string `json:"source"`

	Latest  uint64 `json:"latest"`
	Latency uint64 `json:"latency"`
	Error   string `json:"error"`

	PGLatest  uint64 `json:"pg_latest"`
	PGLatency uint64 `json:"pg_latency"`
	PGError   string `json:"pg_error"`
}

func (h *Handler) Prom(w http.ResponseWriter, r *http.Request) {
	h.diagLastReqMut.Lock()
	if time.Since(h.diagLastReq) < time.Second {
		h.diagLastReqMut.Unlock()
		slog.InfoContext(r.Context(), "rate limiting metrics")
		fmt.Fprintf(w, "too many diag requests")
		return
	}
	h.diagLastReq = time.Now()
	h.diagLastReqMut.Unlock()

	promhttp.Handler().ServeHTTP(w, r)

	checkSource := func(srcName string, src shovel.Source) []string {
		var (
			res                 []string
			start               = time.Now()
			srcLatest, pgLatest uint64
			pgErr, srcErr       int
		)
		// PG
		const q = `
			select num
			from shovel.task_updates
			where src_name = $1
			order by num desc
			limit 1
		`
		err := h.pgp.QueryRow(r.Context(), q, srcName).Scan(&pgLatest)
		if err != nil {
			pgErr++
		}
		res = append(res, "# HELP shovel_latest_block_local last block processed")
		res = append(res, "# TYPE shovel_latest_block_local gauge")
		res = append(res, fmt.Sprintf(`shovel_latest_block_local{src="%s"} %d`, srcName, pgLatest))

		res = append(res, "# HELP shovel_pg_ping number of ms to make basic status query")
		res = append(res, "# TYPE shovel_pg_ping gauge")
		res = append(res, fmt.Sprintf(`shovel_pg_ping %d`, uint64(time.Since(start)/time.Millisecond)))

		res = append(res, "# HELP shovel_pg_ping_error number of errors in making basic status query")
		res = append(res, "# TYPE shovel_pg_ping_error gauge")
		res = append(res, fmt.Sprintf(`shovel_pg_ping_error %d`, pgErr))

		// Source
		start = time.Now()
		srcLatest, _, err = src.Latest(r.Context(), src.NextURL().String(), 0)
		if err != nil {
			srcErr++
		}
		res = append(res, "# HELP shovel_latest_block_remote latest block height from rpc api")
		res = append(res, "# TYPE shovel_latest_block_remote gauge")
		res = append(res, fmt.Sprintf(`shovel_latest_block_remote{src="%s"} %d`, srcName, srcLatest))

		res = append(res, "# HELP shovel_rpc_ping number of ms to make a basic http request to rpc api")
		res = append(res, "# TYPE shovel_rpc_ping gauge")
		res = append(res, fmt.Sprintf(`shovel_rpc_ping{src="%s"} %d`, srcName, uint64(time.Since(start)/time.Millisecond)))

		res = append(res, "# HELP shovel_rpc_ping_error number of errors in making basic rpc api request")
		res = append(res, "# TYPE shovel_rpc_ping_error gauge")
		res = append(res, fmt.Sprintf(`shovel_rpc_ping_error{src="%s"} %d`, srcName, srcErr))

		// Delta
		res = append(res, "# HELP shovel_delta number of blocks between the source and the shovel database")
		res = append(res, "# TYPE shovel_delta gauge")
		res = append(res, fmt.Sprintf(`shovel_delta{src="%s"} %d`, srcName, srcLatest-pgLatest))
		return res
	}

	scs, err := h.conf.AllSources(r.Context(), h.pgp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var res []string
	for _, sc := range scs {
		src := jrpc2.New(sc.URLs...)
		for _, line := range checkSource(sc.Name, src) {
			res = append(res, line)
		}
	}
	fmt.Fprintf(w, strings.Join(res, "\n"))
}

func (h *Handler) Diag(w http.ResponseWriter, r *http.Request) {
	h.diagLastReqMut.Lock()
	if time.Since(h.diagLastReq) < time.Second {
		h.diagLastReqMut.Unlock()
		slog.InfoContext(r.Context(), "rate limiting metrics")
		fmt.Fprintf(w, "too many diag requests")
		return
	}
	h.diagLastReq = time.Now()
	h.diagLastReqMut.Unlock()

	var (
		res []DiagResult
		ctx = r.Context()
	)
	checkPG := func(dr *DiagResult) {
		start := time.Now()
		const q = `
			select num
			from shovel.task_updates
			where src_name = $1
			order by num desc
			limit 1
		`
		err := h.pgp.QueryRow(ctx, q, dr.Source).Scan(&dr.PGLatest)
		dr.PGLatency = uint64(time.Since(start) / time.Millisecond)
		if err != nil {
			dr.PGError = err.Error()
		}
	}
	checkSrc := func(src shovel.Source, dr *DiagResult) {
		start := time.Now()
		n, _, err := src.Latest(ctx, src.NextURL().String(), 0)
		if err != nil {
			dr.Error = err.Error()
		}
		dr.Latest = n
		dr.Latency = uint64(time.Since(start) / time.Millisecond)
	}
	scs, err := h.conf.AllSources(ctx, h.pgp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, sc := range scs {
		var (
			dr  = &DiagResult{Source: sc.Name}
			src = jrpc2.New(sc.URLs...)
		)
		checkPG(dr)
		checkSrc(src, dr)
		res = append(res, *dr)
	}
	if err := json.NewEncoder(w).Encode(res); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
	}
}

func (h *Handler) template(local bool, name string) (*template.Template, error) {
	if local {
		b, err := os.ReadFile(fmt.Sprintf("./shovel/web/%s.html", name))
		if err == nil {
			return template.New(name).Parse(string(b))
		}
		slog.Info("using pre-compiled html templates")
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
		slog.ErrorContext(ctx, "decoding integration", "error", err)
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
		slog.ErrorContext(ctx, "encoding integration", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	const q = `insert into shovel.integrations(name, conf) values ($1, $2)`
	_, err = h.pgp.Exec(ctx, q, ig.Name, cj)
	if err != nil {
		slog.ErrorContext(ctx, "inserting integration", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := h.mgr.Restart(); err != nil {
		slog.ErrorContext(ctx, "saving integration", "error", err)
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
	tmpl, err := h.template(isLoopback(r), "add-integration")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmpl.Execute(w, view); err != nil {
		slog.ErrorContext(ctx, "template", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

type IndexView struct {
	SourceUpdates []shovel.SrcUpdate
	TaskUpdates   map[string][]shovel.TaskUpdate
}

func (h *Handler) Index(w http.ResponseWriter, r *http.Request) {
	var (
		ctx  = r.Context()
		view = IndexView{}
		err  error
	)
	view.SourceUpdates, err = shovel.SourceUpdates(ctx, h.pgp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	tus, err := shovel.TaskUpdates(ctx, h.pgp)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	view.TaskUpdates = make(map[string][]shovel.TaskUpdate)
	for _, tu := range tus {
		view.TaskUpdates[tu.SrcName] = append(
			view.TaskUpdates[tu.SrcName],
			tu,
		)
	}
	t, err := h.template(isLoopback(r), "index")
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
	t, err := h.template(isLoopback(r), "add-source")
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
		slog.ErrorContext(ctx, "parsing chain id", "error", err)
		return
	}
	name := r.FormValue("name")
	if len(name) == 0 {
		slog.ErrorContext(ctx, "parsing chain name", "error", err)
		return
	}
	if err := wstrings.Safe(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	url := r.FormValue("ethURL")
	if len(url) == 0 {
		slog.ErrorContext(ctx, "parsing chain eth url", "error", err)
		return
	}
	const q = `
		insert into shovel.sources(chain_id, name, url)
		values ($1, $2, $3)
	`
	_, err = h.pgp.Exec(ctx, q, chainID, name, url)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		slog.ErrorContext(ctx, "inserting task", "error", err)
		return
	}
	if err := h.mgr.Restart(); err != nil {
		slog.ErrorContext(ctx, "saving source", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
