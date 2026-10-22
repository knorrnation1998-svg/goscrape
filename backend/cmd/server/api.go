package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/goscrape/backend/internal/crawler"
	"github.com/goscrape/backend/internal/storage"
)

// api holds the dependencies shared by every HTTP handler.
type api struct {
	store  *storage.Store
	crawls *crawler.Manager
	log    *slog.Logger
}

// routes builds the HTTP handler. Go 1.22+ method-aware patterns are used, so
// no third-party router is needed.
func (a *api) routes(corsOrigin string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/crawl", a.startCrawl)
	mux.HandleFunc("GET /api/crawls", a.listCrawls)
	mux.HandleFunc("GET /api/crawls/{id}", a.getCrawl)
	mux.HandleFunc("POST /api/crawls/{id}/cancel", a.cancelCrawl)
	mux.HandleFunc("GET /api/stats", a.stats)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	return chain(mux, a.recoverer, a.logging, cors(corsOrigin))
}

type startRequest struct {
	URL      string `json:"url"`
	MaxDepth *int   `json:"maxDepth"` // optional, defaults to 1
}

// POST /api/crawl
func (a *api) startCrawl(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req startRequest
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, `body must be JSON like {"url": "https://example.com", "maxDepth": 2}`)
		return
	}
	depth := 1
	if req.MaxDepth != nil {
		depth = *req.MaxDepth
	}

	id, err := a.crawls.Start(r.Context(), req.URL, depth)
	switch {
	case errors.Is(err, crawler.ErrInvalidRequest):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, crawler.ErrShuttingDown):
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	case err != nil:
		a.log.Error("start crawl", "err", err)
		writeError(w, http.StatusInternalServerError, "could not start crawl")
		return
	}

	job, err := a.store.GetJob(r.Context(), id)
	if err != nil {
		a.log.Error("load new job", "job", id, "err", err)
		writeError(w, http.StatusInternalServerError, "crawl started but could not be loaded")
		return
	}
	job.ActiveWorkers = a.crawls.ActiveWorkers(id)
	writeJSON(w, http.StatusCreated, job)
}

// GET /api/crawls?limit=50
func (a *api) listCrawls(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50, 1, 200)
	jobs, err := a.store.ListJobs(r.Context(), limit)
	if err != nil {
		a.log.Error("list jobs", "err", err)
		writeError(w, http.StatusInternalServerError, "could not list crawls")
		return
	}
	for i := range jobs {
		jobs[i].ActiveWorkers = a.crawls.ActiveWorkers(jobs[i].ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"crawls": jobs})
}

type crawlDetail struct {
	Job   *storage.Job   `json:"job"`
	Items []storage.Item `json:"items"`
}

// GET /api/crawls/{id}?afterId=0&limit=5000
//
// afterId makes polling cheap: a client passes the highest item ID it already
// holds and receives only newer items.
func (a *api) getCrawl(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	job, err := a.store.GetJob(r.Context(), id)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("crawl %d not found", id))
		return
	}
	if err != nil {
		a.log.Error("get job", "job", id, "err", err)
		writeError(w, http.StatusInternalServerError, "could not load crawl")
		return
	}

	afterID := int64(queryInt(r, "afterId", 0, 0, math.MaxInt32))
	limit := queryInt(r, "limit", 5000, 1, 20000)
	items, err := a.store.ListItems(r.Context(), id, afterID, limit)
	if err != nil {
		a.log.Error("list items", "job", id, "err", err)
		writeError(w, http.StatusInternalServerError, "could not load crawl items")
		return
	}
	job.ActiveWorkers = a.crawls.ActiveWorkers(id)
	writeJSON(w, http.StatusOK, crawlDetail{Job: job, Items: items})
}

// POST /api/crawls/{id}/cancel
func (a *api) cancelCrawl(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	job, err := a.store.GetJob(r.Context(), id)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("crawl %d not found", id))
		return
	}
	if err != nil {
		a.log.Error("get job", "job", id, "err", err)
		writeError(w, http.StatusInternalServerError, "could not load crawl")
		return
	}
	if !a.crawls.Cancel(id) {
		writeError(w, http.StatusConflict, "crawl is not running")
		return
	}
	job.ActiveWorkers = a.crawls.ActiveWorkers(id)
	writeJSON(w, http.StatusAccepted, job)
}

type statsResponse struct {
	storage.GlobalStats
	ActiveWorkers int `json:"activeWorkers"`
	RunningJobs   int `json:"runningJobs"`
}

// GET /api/stats
func (a *api) stats(w http.ResponseWriter, r *http.Request) {
	g, err := a.store.GlobalStats(r.Context())
	if err != nil {
		a.log.Error("global stats", "err", err)
		writeError(w, http.StatusInternalServerError, "could not compute stats")
		return
	}
	running, active := a.crawls.Snapshot()
	writeJSON(w, http.StatusOK, statsResponse{GlobalStats: g, ActiveWorkers: active, RunningJobs: running})
}

// ---- helpers ---------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode response", "err", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "id must be a positive integer")
		return 0, false
	}
	return id, true
}

// queryInt reads an integer query parameter, falling back to def when it is
// missing or malformed and clamping the result to [lo, hi].
func queryInt(r *http.Request, key string, def, lo, hi int) int {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

// ---- middleware ------------------------------------------------------------

type middleware func(http.Handler) http.Handler

// chain wraps h so that the first middleware listed is the outermost.
func chain(h http.Handler, mws ...middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (a *api) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		a.log.Info("http", "method", r.Method, "path", r.URL.Path, "status", rec.status,
			"dur", time.Since(start).Round(time.Microsecond))
	})
}

func (a *api) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				a.log.Error("panic in handler", "panic", rec, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// cors allows a single browser origin to call the API directly. The bundled
// frontend proxies /api through Next.js instead, so this is only needed when
// the browser talks to the Go server cross-origin. An empty origin disables it.
func cors(origin string) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if origin != "" {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Vary", "Origin")
				h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Content-Type")
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
