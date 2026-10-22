package crawler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/goscrape/backend/internal/parser"
	"github.com/goscrape/backend/internal/storage"
)

// ---- fakes -----------------------------------------------------------------

// fakeSite serves an in-memory link graph. Unknown URLs return 404.
type fakeSite struct {
	root  string
	pages map[string][]string
	delay time.Duration   // applied to every URL except root
	fail  map[string]bool // URLs that return a transport error

	mu      sync.Mutex
	fetches map[string]int
}

func (f *fakeSite) Fetch(ctx context.Context, rawURL string) (*parser.Page, error) {
	f.mu.Lock()
	if f.fetches == nil {
		f.fetches = map[string]int{}
	}
	f.fetches[rawURL]++
	f.mu.Unlock()

	if f.delay > 0 && rawURL != f.root {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.fail[rawURL] {
		return nil, errors.New("connection refused")
	}
	links, ok := f.pages[rawURL]
	status := 200
	if !ok {
		status = 404
	}
	return &parser.Page{
		URL:         rawURL,
		StatusCode:  status,
		ContentType: "text/html",
		Title:       "title of " + rawURL,
		Meta:        map[string]string{},
		Headings:    []parser.Heading{},
		Links:       links,
		Bytes:       128,
		Duration:    5 * time.Millisecond,
	}, nil
}

func (f *fakeSite) fetchCount(u string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fetches[u]
}

type fakeRepo struct {
	mu     sync.Mutex
	nextID int64
	items  []storage.Item
	status map[int64]storage.Status
	errMsg map[int64]string
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{status: map[int64]storage.Status{}, errMsg: map[int64]string{}}
}

func (r *fakeRepo) CreateJob(_ context.Context, _ string, _ int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	r.status[r.nextID] = storage.StatusPending
	return r.nextID, nil
}

func (r *fakeRepo) MarkRunning(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status[id] = storage.StatusRunning
	return nil
}

func (r *fakeRepo) InsertItem(_ context.Context, it *storage.Item) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items = append(r.items, *it)
	return nil
}

func (r *fakeRepo) FinishJob(_ context.Context, id int64, s storage.Status, msg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.status[id] = s
	r.errMsg[id] = msg
	return nil
}

func (r *fakeRepo) snapshot(id int64) (storage.Status, string, []storage.Item) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.status[id], r.errMsg[id], append([]storage.Item(nil), r.items...)
}

func (r *fakeRepo) urls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.items))
	for _, it := range r.items {
		out = append(out, it.URL)
	}
	sort.Strings(out)
	return out
}

// ---- helpers ---------------------------------------------------------------

func newTestManager(t *testing.T, site Fetcher, cfg Config, rps float64, burst int) (*Manager, *fakeRepo) {
	t.Helper()
	repo := newFakeRepo()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := NewManager(context.Background(), repo, site, NewDomainLimiter(rps, burst), cfg, log)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = m.Shutdown(ctx)
	})
	return m, repo
}

func waitTerminal(t *testing.T, repo *fakeRepo, id int64) storage.Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		s, _, _ := repo.snapshot(id)
		switch s {
		case storage.StatusCompleted, storage.StatusFailed, storage.StatusCancelled:
			return s
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job %d did not reach a terminal state in time", id)
	return ""
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

const root = "http://site.test/"

// ---- tests -----------------------------------------------------------------

func TestCrawlFollowsInternalLinksUpToMaxDepth(t *testing.T) {
	site := &fakeSite{root: root, pages: map[string][]string{
		root:                 {"http://site.test/a", "http://site.test/b", "http://other.test/x", "http://site.test/logo.png"},
		"http://site.test/a": {"http://site.test/c", root},
		"http://site.test/b": {"http://site.test/c"},
		"http://site.test/c": {"http://site.test/d"},
		"http://site.test/d": {},
	}}
	m, repo := newTestManager(t, site, Config{Workers: 4, MaxPages: 100}, 1000, 1000)

	id, err := m.Start(context.Background(), "http://site.test", 2)
	if err != nil {
		t.Fatal(err)
	}
	if s := waitTerminal(t, repo, id); s != storage.StatusCompleted {
		t.Fatalf("status = %s", s)
	}

	want := []string{root, "http://site.test/a", "http://site.test/b", "http://site.test/c"}
	if got := repo.urls(); !sameStrings(got, want) {
		t.Fatalf("crawled %v, want %v", got, want)
	}
	if n := site.fetchCount("http://site.test/c"); n != 1 {
		t.Fatalf("/c fetched %d times, want exactly 1 (dedupe)", n)
	}
	if n := site.fetchCount("http://other.test/x"); n != 0 {
		t.Fatalf("external link was fetched")
	}
	if n := site.fetchCount("http://site.test/logo.png"); n != 0 {
		t.Fatalf("binary asset was fetched")
	}
}

func TestCrawlDepthZeroFetchesOnlyTheStartPage(t *testing.T) {
	site := &fakeSite{root: root, pages: map[string][]string{
		root: {"http://site.test/a"}, "http://site.test/a": {},
	}}
	m, repo := newTestManager(t, site, Config{Workers: 2, MaxPages: 100}, 1000, 1000)

	id, err := m.Start(context.Background(), root, 0)
	if err != nil {
		t.Fatal(err)
	}
	waitTerminal(t, repo, id)
	if got := repo.urls(); !sameStrings(got, []string{root}) {
		t.Fatalf("crawled %v, want only the root", got)
	}
}

func TestCrawlStopsAtMaxPages(t *testing.T) {
	links := make([]string, 0, 50)
	pages := map[string][]string{}
	for i := 0; i < 50; i++ {
		u := fmt.Sprintf("http://site.test/p%d", i)
		links = append(links, u)
		pages[u] = nil
	}
	pages[root] = links
	site := &fakeSite{root: root, pages: pages}
	m, repo := newTestManager(t, site, Config{Workers: 4, MaxPages: 10}, 1000, 1000)

	id, err := m.Start(context.Background(), root, 3)
	if err != nil {
		t.Fatal(err)
	}
	if s := waitTerminal(t, repo, id); s != storage.StatusCompleted {
		t.Fatalf("status = %s", s)
	}
	if n := len(repo.urls()); n != 10 {
		t.Fatalf("fetched %d pages, want exactly the MaxPages budget of 10", n)
	}
}

func TestCrawlCancelAbortsMidExecution(t *testing.T) {
	links := make([]string, 0, 30)
	pages := map[string][]string{}
	for i := 0; i < 30; i++ {
		u := fmt.Sprintf("http://site.test/p%d", i)
		links = append(links, u)
		pages[u] = nil
	}
	pages[root] = links
	site := &fakeSite{root: root, pages: pages, delay: 300 * time.Millisecond}
	m, repo := newTestManager(t, site, Config{Workers: 2, MaxPages: 100}, 1000, 1000)

	id, err := m.Start(context.Background(), root, 1)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond) // root is instant, so workers are now mid-fetch
	if n := m.ActiveWorkers(id); n < 1 {
		t.Fatalf("expected busy workers mid-crawl, got %d", n)
	}

	start := time.Now()
	if !m.Cancel(id) {
		t.Fatal("Cancel reported the job as not running")
	}
	if s := waitTerminal(t, repo, id); s != storage.StatusCancelled {
		t.Fatalf("status = %s, want cancelled", s)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("cancellation took %v; in-flight fetches should abort immediately", d)
	}
	if n := len(repo.urls()); n >= 31 {
		t.Fatalf("crawl was not actually aborted: %d pages stored", n)
	}

	// The run is deregistered shortly after it records its final state.
	deadline := time.Now().Add(2 * time.Second)
	for {
		running, active := m.Snapshot()
		if running == 0 && active == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job still registered after cancel: running=%d active=%d", running, active)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if m.Cancel(id) {
		t.Fatal("Cancel on a finished job should return false")
	}
}

func TestCrawlFailsWhenStartPageUnreachable(t *testing.T) {
	site := &fakeSite{root: root, fail: map[string]bool{root: true}}
	m, repo := newTestManager(t, site, Config{Workers: 2, MaxPages: 100}, 1000, 1000)

	id, err := m.Start(context.Background(), root, 2)
	if err != nil {
		t.Fatal(err)
	}
	if s := waitTerminal(t, repo, id); s != storage.StatusFailed {
		t.Fatalf("status = %s, want failed", s)
	}
	_, msg, items := repo.snapshot(id)
	if msg != "connection refused" {
		t.Fatalf("job error = %q", msg)
	}
	if len(items) != 1 || items[0].Error == "" {
		t.Fatalf("want one errored item, got %+v", items)
	}
}

func TestCrawlSurvivesFailingSubpage(t *testing.T) {
	site := &fakeSite{
		root:  root,
		pages: map[string][]string{root: {"http://site.test/ok", "http://site.test/bad"}, "http://site.test/ok": {}},
		fail:  map[string]bool{"http://site.test/bad": true},
	}
	m, repo := newTestManager(t, site, Config{Workers: 2, MaxPages: 100}, 1000, 1000)

	id, err := m.Start(context.Background(), root, 1)
	if err != nil {
		t.Fatal(err)
	}
	if s := waitTerminal(t, repo, id); s != storage.StatusCompleted {
		t.Fatalf("status = %s, want completed despite one failing page", s)
	}
	if n := len(repo.urls()); n != 3 {
		t.Fatalf("stored %d items, want 3", n)
	}
}

func TestCrawlHonoursDomainRateLimit(t *testing.T) {
	pages := map[string][]string{root: {"http://site.test/a", "http://site.test/b", "http://site.test/c", "http://site.test/d"}}
	for _, p := range pages[root] {
		pages[p] = nil
	}
	site := &fakeSite{root: root, pages: pages}
	// 20 req/s with no burst headroom: five requests need at least 4 x 50ms.
	m, repo := newTestManager(t, site, Config{Workers: 8, MaxPages: 100}, 20, 1)

	start := time.Now()
	id, err := m.Start(context.Background(), root, 1)
	if err != nil {
		t.Fatal(err)
	}
	waitTerminal(t, repo, id)
	if d := time.Since(start); d < 180*time.Millisecond {
		t.Fatalf("5 requests finished in %v despite the 20 req/s limit", d)
	}
}

func TestShutdownCancelsRunningJobsAndRejectsNewOnes(t *testing.T) {
	pages := map[string][]string{root: {"http://site.test/a", "http://site.test/b"}}
	site := &fakeSite{root: root, pages: pages, delay: 2 * time.Second}
	m, repo := newTestManager(t, site, Config{Workers: 2, MaxPages: 100}, 1000, 1000)

	id, err := m.Start(context.Background(), root, 1)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := m.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if s, _, _ := repo.snapshot(id); s != storage.StatusCancelled {
		t.Fatalf("status after shutdown = %s, want cancelled", s)
	}
	if _, err := m.Start(context.Background(), root, 1); !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("Start after shutdown: err = %v, want ErrShuttingDown", err)
	}
}

func TestValidateRequest(t *testing.T) {
	tests := []struct {
		url     string
		depth   int
		want    string
		wantErr bool
	}{
		{"example.com", 1, "https://example.com/", false},
		{"  http://Example.com/docs  ", 0, "http://example.com/docs", false},
		{"https://example.com", MaxDepthLimit, "https://example.com/", false},
		{"", 1, "", true},
		{"ftp://example.com", 1, "", true},
		{"http://", 1, "", true},
		{"https://example.com", -1, "", true},
		{"https://example.com", MaxDepthLimit + 1, "", true},
	}
	for _, tc := range tests {
		got, err := ValidateRequest(tc.url, tc.depth)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateRequest(%q, %d) err = %v, wantErr %v", tc.url, tc.depth, err, tc.wantErr)
			continue
		}
		if tc.wantErr && !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("ValidateRequest(%q, %d) err = %v, want ErrInvalidRequest", tc.url, tc.depth, err)
		}
		if got != tc.want {
			t.Errorf("ValidateRequest(%q, %d) = %q, want %q", tc.url, tc.depth, got, tc.want)
		}
	}
}
