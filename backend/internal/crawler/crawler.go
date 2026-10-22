// Package crawler runs concurrent, rate-limited, abortable crawls.
//
// Each job is driven by a single coordinator goroutine that owns the frontier
// (a BFS queue) and the visited set, so neither needs a lock. It hands tasks to
// a pool of worker goroutines over one channel and receives their results over
// another. Workers fetch pages (waiting on the per-domain token bucket first)
// and persist them; the coordinator decides which discovered links to follow.
// Cancelling the job context unwinds the coordinator and every worker.
package crawler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/goscrape/backend/internal/parser"
	"github.com/goscrape/backend/internal/storage"
)

const (
	// MaxDepthLimit is the deepest crawl a request may ask for.
	MaxDepthLimit = 5

	storedLinksPerPage = 200
	dbTimeout          = 10 * time.Second
)

var (
	// ErrInvalidRequest is returned (wrapped) for bad URLs or depths.
	ErrInvalidRequest = errors.New("invalid crawl request")
	// ErrShuttingDown is returned when the manager no longer accepts jobs.
	ErrShuttingDown = errors.New("crawler is shutting down")
)

// Fetcher downloads and parses one page. *parser.Fetcher implements it.
type Fetcher interface {
	Fetch(ctx context.Context, rawURL string) (*parser.Page, error)
}

// Repository is the persistence the crawler needs. *storage.Store implements it.
type Repository interface {
	CreateJob(ctx context.Context, rawURL string, maxDepth int) (int64, error)
	MarkRunning(ctx context.Context, id int64) error
	InsertItem(ctx context.Context, item *storage.Item) error
	FinishJob(ctx context.Context, id int64, status storage.Status, message string) error
}

// Config tunes every crawl started by a Manager.
type Config struct {
	Workers  int // concurrent fetch workers per job
	MaxPages int // hard cap on pages fetched per job
}

func (c Config) withDefaults() Config {
	if c.Workers < 1 {
		c.Workers = 8
	}
	if c.MaxPages < 1 {
		c.MaxPages = 500
	}
	return c
}

// ValidateRequest checks a crawl request and returns the normalised start URL.
func ValidateRequest(rawURL string, maxDepth int) (string, error) {
	if maxDepth < 0 || maxDepth > MaxDepthLimit {
		return "", fmt.Errorf("%w: maxDepth must be between 0 and %d", ErrInvalidRequest, MaxDepthLimit)
	}
	target, err := parser.NormalizeTarget(rawURL)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	return target, nil
}

// Manager starts, tracks and cancels crawl jobs.
type Manager struct {
	repo    Repository
	fetcher Fetcher
	limiter *DomainLimiter
	cfg     Config
	log     *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu     sync.RWMutex
	runs   map[int64]*run
	closed bool
}

// NewManager creates a Manager. Cancelling parent aborts every running job.
func NewManager(parent context.Context, repo Repository, fetcher Fetcher, limiter *DomainLimiter, cfg Config, log *slog.Logger) *Manager {
	if log == nil {
		log = slog.Default()
	}
	ctx, cancel := context.WithCancel(parent)
	return &Manager{
		repo:    repo,
		fetcher: fetcher,
		limiter: limiter,
		cfg:     cfg.withDefaults(),
		log:     log,
		ctx:     ctx,
		cancel:  cancel,
		runs:    make(map[int64]*run),
	}
}

// Start validates the request, records a job and begins crawling it in the
// background. The job's lifetime is tied to the manager, not to ctx, which is
// only used for the initial database write.
func (m *Manager) Start(ctx context.Context, rawURL string, maxDepth int) (int64, error) {
	target, err := ValidateRequest(rawURL, maxDepth)
	if err != nil {
		return 0, err
	}
	root, err := url.Parse(target)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}

	id, err := m.repo.CreateJob(ctx, target, maxDepth)
	if err != nil {
		return 0, fmt.Errorf("create job: %w", err)
	}

	jobCtx, cancel := context.WithCancel(m.ctx)
	r := &run{m: m, id: id, rootURL: target, root: root, maxDepth: maxDepth, cancel: cancel}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		cancel()
		m.finish(id, storage.StatusCancelled, "server shutting down")
		return 0, ErrShuttingDown
	}
	m.runs[id] = r
	m.wg.Add(1)
	m.mu.Unlock()

	go func() {
		defer m.wg.Done()
		defer cancel()
		r.execute(jobCtx)
		m.mu.Lock()
		delete(m.runs, id)
		m.mu.Unlock()
	}()
	return id, nil
}

// Cancel aborts a running job. It returns false if the job is not running.
// The job's status becomes "cancelled" once its workers have unwound.
func (m *Manager) Cancel(id int64) bool {
	m.mu.RLock()
	r, ok := m.runs[id]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	r.cancel()
	return true
}

// ActiveWorkers returns how many workers of the job are fetching right now.
func (m *Manager) ActiveWorkers(id int64) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if r, ok := m.runs[id]; ok {
		return int(r.active.Load())
	}
	return 0
}

// Snapshot returns the number of running jobs and busy workers across them.
func (m *Manager) Snapshot() (runningJobs, activeWorkers int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, r := range m.runs {
		runningJobs++
		activeWorkers += int(r.active.Load())
	}
	return runningJobs, activeWorkers
}

// Shutdown stops accepting jobs, cancels the running ones and waits for them
// to record their final state or for ctx to expire.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	m.cancel()

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// finish writes a terminal job state using a context that outlives
// cancellation, so aborted jobs are still recorded.
func (m *Manager) finish(id int64, status storage.Status, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	if err := m.repo.FinishJob(ctx, id, status, message); err != nil {
		m.log.Error("finish job", "job", id, "status", status, "err", err)
	}
}

// run is the state of one crawl job.
type run struct {
	m        *Manager
	id       int64
	rootURL  string
	root     *url.URL
	maxDepth int
	cancel   context.CancelFunc
	active   atomic.Int32 // workers currently fetching
}

type task struct {
	url   string
	depth int
}

type result struct {
	task  task
	links []string // links worth following, already absolute
	err   string   // transport-level failure; empty if a response was received
}

type outcome struct {
	fetched int    // pages for which a response was received
	rootErr string // failure of the start URL, if any
}

func (r *run) execute(ctx context.Context) {
	m := r.m
	startCtx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	err := m.repo.MarkRunning(startCtx, r.id)
	cancel()
	if err != nil {
		m.log.Error("mark running", "job", r.id, "err", err)
		m.finish(r.id, storage.StatusFailed, "could not start job: "+err.Error())
		return
	}
	m.log.Info("crawl started", "job", r.id, "url", r.rootURL, "depth", r.maxDepth)

	out := r.crawl(ctx)

	switch {
	case ctx.Err() != nil:
		m.finish(r.id, storage.StatusCancelled, "")
		m.log.Info("crawl cancelled", "job", r.id)
	case out.fetched == 0 && out.rootErr != "":
		m.finish(r.id, storage.StatusFailed, out.rootErr)
		m.log.Warn("crawl failed", "job", r.id, "err", out.rootErr)
	default:
		m.finish(r.id, storage.StatusCompleted, "")
		m.log.Info("crawl completed", "job", r.id, "pages", out.fetched)
	}
}

// crawl is the coordinator. It is the only goroutine that touches the frontier
// and the visited set.
func (r *run) crawl(ctx context.Context) outcome {
	cfg := r.m.cfg
	tasks := make(chan task)
	results := make(chan result)

	var workers sync.WaitGroup
	for i := 0; i < cfg.Workers; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			r.worker(ctx, tasks, results)
		}()
	}

	var out outcome
	visited := map[string]struct{}{r.rootURL: {}}
	frontier := []task{{url: r.rootURL, depth: 0}}
	scheduled, inflight := 0, 0

loop:
	for inflight > 0 || (len(frontier) > 0 && scheduled < cfg.MaxPages) {
		// A nil send channel disables that select case, so tasks are only
		// offered while the frontier is non-empty and the page budget lasts.
		var (
			send chan<- task
			next task
		)
		if len(frontier) > 0 && scheduled < cfg.MaxPages {
			send, next = tasks, frontier[0]
		}

		select {
		case <-ctx.Done():
			break loop

		case send <- next:
			frontier = frontier[1:]
			scheduled++
			inflight++

		case res := <-results:
			inflight--
			if res.err == "" {
				out.fetched++
			} else if res.task.depth == 0 {
				out.rootErr = res.err
			}
			if res.task.depth >= r.maxDepth {
				continue
			}
			for _, link := range res.links {
				if scheduled+len(frontier) >= cfg.MaxPages {
					break
				}
				if _, seen := visited[link]; seen {
					continue
				}
				if !r.eligible(link) {
					continue
				}
				visited[link] = struct{}{}
				frontier = append(frontier, task{url: link, depth: res.task.depth + 1})
			}
		}
	}

	close(tasks)
	workers.Wait()
	return out
}

// worker fetches tasks until the channel is closed or ctx is cancelled.
func (r *run) worker(ctx context.Context, tasks <-chan task, results chan<- result) {
	for t := range tasks {
		res, ok := r.process(ctx, t)
		if !ok {
			continue // cancelled mid-fetch; the coordinator is already unwinding
		}
		select {
		case results <- res:
		case <-ctx.Done():
			return
		}
	}
}

// process rate-limits, fetches and stores one page. The boolean is false when
// the job was cancelled before a result could be produced.
func (r *run) process(ctx context.Context, t task) (result, bool) {
	r.active.Add(1)
	defer r.active.Add(-1)

	u, err := url.Parse(t.url)
	if err != nil {
		res := result{task: t, err: err.Error()}
		r.store(&storage.Item{JobID: r.id, URL: t.url, Path: t.url, Depth: t.depth, Error: res.err, FetchedAt: time.Now().UTC()})
		return res, true
	}

	if err := r.m.limiter.Wait(ctx, u.Host); err != nil {
		return result{}, false
	}

	start := time.Now()
	page, fetchErr := r.m.fetcher.Fetch(ctx, t.url)
	if ctx.Err() != nil {
		return result{}, false
	}

	item := &storage.Item{
		JobID:     r.id,
		URL:       u.String(),
		Path:      u.RequestURI(),
		Depth:     t.depth,
		FetchedAt: time.Now().UTC(),
	}
	res := result{task: t}

	if fetchErr != nil {
		item.Error = fetchErr.Error()
		item.DurationMs = time.Since(start).Milliseconds()
		res.err = item.Error
	} else {
		if final, err := url.Parse(page.URL); err == nil {
			item.URL = final.String()
			item.Path = final.RequestURI()
		}
		item.StatusCode = page.StatusCode
		item.Title = page.Title
		item.Meta = page.Meta
		item.Headings = page.Headings
		item.Links = firstN(page.Links, storedLinksPerPage)
		item.LinkCount = len(page.Links)
		item.ContentType = page.ContentType
		item.Bytes = page.Bytes
		item.DurationMs = page.Duration.Milliseconds()
		if page.StatusCode < 400 {
			res.links = page.Links
		}
	}

	r.store(item)
	return res, true
}

// store persists an item. It uses a fresh context so pages that were fetched
// just before a cancellation are not lost.
func (r *run) store(item *storage.Item) {
	ctx, cancel := context.WithTimeout(context.Background(), dbTimeout)
	defer cancel()
	if err := r.m.repo.InsertItem(ctx, item); err != nil {
		r.m.log.Error("store item", "job", r.id, "url", item.URL, "err", err)
	}
}

// eligible reports whether link should be crawled: same site as the start URL
// and not an obvious binary or static asset.
func (r *run) eligible(link string) bool {
	u, err := url.Parse(link)
	if err != nil {
		return false
	}
	if !sameSite(r.root.Host, u.Host) {
		return false
	}
	return !skippedExtensions[strings.ToLower(path.Ext(u.Path))]
}

func sameSite(a, b string) bool {
	return trimWWW(strings.ToLower(a)) == trimWWW(strings.ToLower(b))
}

func trimWWW(host string) string { return strings.TrimPrefix(host, "www.") }

var skippedExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".svg": true, ".ico": true,
	".avif": true, ".bmp": true, ".pdf": true, ".zip": true, ".gz": true, ".tar": true, ".rar": true,
	".7z": true, ".mp3": true, ".mp4": true, ".avi": true, ".mov": true, ".webm": true, ".woff": true,
	".woff2": true, ".ttf": true, ".eot": true, ".css": true, ".js": true, ".json": true, ".xml": true,
	".exe": true, ".dmg": true, ".iso": true, ".csv": true, ".xls": true, ".xlsx": true, ".doc": true,
	".docx": true, ".ppt": true, ".pptx": true,
}

func firstN[T any](s []T, n int) []T {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
