// Package storage persists crawl jobs and the data extracted from each page
// in a SQLite database.
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, no CGO required

	"github.com/goscrape/backend/internal/parser"
)

// ErrNotFound is returned when a requested row does not exist.
var ErrNotFound = errors.New("storage: not found")

// Status is the lifecycle state of a crawl job.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCancelled Status = "cancelled"
)

// Job is a crawl job together with its summary metrics. The metrics are
// aggregated from crawl_items at query time, so they are always live.
type Job struct {
	ID           int64      `json:"id"`
	URL          string     `json:"url"`
	MaxDepth     int        `json:"maxDepth"`
	Status       Status     `json:"status"`
	Error        string     `json:"error,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	StartedAt    *time.Time `json:"startedAt,omitempty"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
	PagesScraped int        `json:"pagesScraped"` // fetched without error and status < 400
	PagesFailed  int        `json:"pagesFailed"`  // transport error or status >= 400
	BytesTotal   int64      `json:"bytesTotal"`
	AvgLatencyMs float64    `json:"avgLatencyMs"`

	// ActiveWorkers is not persisted; the API layer fills it in from the
	// in-memory crawl manager.
	ActiveWorkers int `json:"activeWorkers"`
}

// Item is one fetched page.
type Item struct {
	ID          int64             `json:"id"`
	JobID       int64             `json:"jobId"`
	URL         string            `json:"url"`
	Path        string            `json:"path"`
	Depth       int               `json:"depth"`
	StatusCode  int               `json:"statusCode"`
	Title       string            `json:"title"`
	Meta        map[string]string `json:"meta"`
	Headings    []parser.Heading  `json:"headings"`
	Links       []string          `json:"links"`
	LinkCount   int               `json:"linkCount"`
	ContentType string            `json:"contentType"`
	Bytes       int64             `json:"bytes"`
	DurationMs  int64             `json:"durationMs"`
	Error       string            `json:"error,omitempty"`
	FetchedAt   time.Time         `json:"fetchedAt"`
}

// GlobalStats aggregates metrics across every job.
type GlobalStats struct {
	TotalJobs    int     `json:"totalJobs"`
	TotalPages   int     `json:"totalPages"`
	FailedPages  int     `json:"failedPages"`
	TotalBytes   int64   `json:"totalBytes"`
	AvgLatencyMs float64 `json:"avgLatencyMs"`
}

// Store is a SQLite-backed repository. It is safe for concurrent use.
type Store struct {
	db *sql.DB
}

var schema = []string{
	`CREATE TABLE IF NOT EXISTS crawl_jobs (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		url         TEXT    NOT NULL,
		max_depth   INTEGER NOT NULL,
		status      TEXT    NOT NULL,
		error       TEXT    NOT NULL DEFAULT '',
		created_at  INTEGER NOT NULL,
		started_at  INTEGER,
		finished_at INTEGER
	)`,
	`CREATE TABLE IF NOT EXISTS crawl_items (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		job_id        INTEGER NOT NULL REFERENCES crawl_jobs(id) ON DELETE CASCADE,
		url           TEXT    NOT NULL,
		path          TEXT    NOT NULL,
		depth         INTEGER NOT NULL,
		status_code   INTEGER NOT NULL DEFAULT 0,
		title         TEXT    NOT NULL DEFAULT '',
		meta_json     TEXT    NOT NULL DEFAULT '{}',
		headings_json TEXT    NOT NULL DEFAULT '[]',
		links_json    TEXT    NOT NULL DEFAULT '[]',
		link_count    INTEGER NOT NULL DEFAULT 0,
		content_type  TEXT    NOT NULL DEFAULT '',
		bytes         INTEGER NOT NULL DEFAULT 0,
		duration_ms   INTEGER NOT NULL DEFAULT 0,
		error         TEXT    NOT NULL DEFAULT '',
		fetched_at    INTEGER NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_crawl_items_job ON crawl_items(job_id, id)`,
	`CREATE INDEX IF NOT EXISTS idx_crawl_jobs_status ON crawl_jobs(status)`,
}

// Open opens (creating if necessary) the database at path and applies the
// schema. WAL mode lets the API read while the crawler writes.
func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	for _, stmt := range schema {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			db.Close()
			return nil, fmt.Errorf("apply schema: %w", err)
		}
	}
	return &Store{db: db}, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// CreateJob inserts a new pending job and returns its ID.
func (s *Store) CreateJob(ctx context.Context, rawURL string, maxDepth int) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO crawl_jobs (url, max_depth, status, created_at) VALUES (?, ?, ?, ?)`,
		rawURL, maxDepth, string(StatusPending), time.Now().UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// MarkRunning moves a job to the running state and stamps its start time.
func (s *Store) MarkRunning(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE crawl_jobs SET status = ?, started_at = ? WHERE id = ?`,
		string(StatusRunning), time.Now().UnixMilli(), id)
	return err
}

// FinishJob moves a job to a terminal state.
func (s *Store) FinishJob(ctx context.Context, id int64, status Status, message string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE crawl_jobs SET status = ?, error = ?, finished_at = ? WHERE id = ?`,
		string(status), message, time.Now().UnixMilli(), id)
	return err
}

// FailInterrupted marks jobs left pending/running by a previous process as
// failed. Call it once at startup.
func (s *Store) FailInterrupted(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE crawl_jobs SET status = ?, error = ?, finished_at = ? WHERE status IN (?, ?)`,
		string(StatusFailed), "interrupted by server restart", time.Now().UnixMilli(),
		string(StatusPending), string(StatusRunning))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// InsertItem stores one fetched page and sets item.ID.
func (s *Store) InsertItem(ctx context.Context, it *Item) error {
	if it.Meta == nil {
		it.Meta = map[string]string{}
	}
	if it.Headings == nil {
		it.Headings = []parser.Heading{}
	}
	if it.Links == nil {
		it.Links = []string{}
	}
	meta, err := json.Marshal(it.Meta)
	if err != nil {
		return fmt.Errorf("encode meta: %w", err)
	}
	headings, err := json.Marshal(it.Headings)
	if err != nil {
		return fmt.Errorf("encode headings: %w", err)
	}
	links, err := json.Marshal(it.Links)
	if err != nil {
		return fmt.Errorf("encode links: %w", err)
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO crawl_items
			(job_id, url, path, depth, status_code, title, meta_json, headings_json, links_json,
			 link_count, content_type, bytes, duration_ms, error, fetched_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		it.JobID, it.URL, it.Path, it.Depth, it.StatusCode, it.Title,
		string(meta), string(headings), string(links),
		it.LinkCount, it.ContentType, it.Bytes, it.DurationMs, it.Error,
		it.FetchedAt.UnixMilli())
	if err != nil {
		return err
	}
	it.ID, err = res.LastInsertId()
	return err
}

// jobSelect aggregates per-job metrics from crawl_items.
const jobSelect = `
SELECT j.id, j.url, j.max_depth, j.status, j.error, j.created_at, j.started_at, j.finished_at,
       COALESCE(s.ok, 0), COALESCE(s.failed, 0), COALESCE(s.bytes, 0), COALESCE(s.avg_ms, 0)
FROM crawl_jobs j
LEFT JOIN (
	SELECT job_id,
	       SUM(CASE WHEN error = '' AND status_code < 400 THEN 1 ELSE 0 END)  AS ok,
	       SUM(CASE WHEN error != '' OR status_code >= 400 THEN 1 ELSE 0 END) AS failed,
	       COALESCE(SUM(bytes), 0)                                            AS bytes,
	       AVG(CASE WHEN error = '' THEN duration_ms END)                     AS avg_ms
	FROM crawl_items
	GROUP BY job_id
) s ON s.job_id = j.id`

type scanner interface {
	Scan(dest ...any) error
}

func scanJob(sc scanner) (*Job, error) {
	var (
		j                 Job
		status            string
		created           int64
		started, finished sql.NullInt64
	)
	if err := sc.Scan(&j.ID, &j.URL, &j.MaxDepth, &status, &j.Error, &created, &started, &finished,
		&j.PagesScraped, &j.PagesFailed, &j.BytesTotal, &j.AvgLatencyMs); err != nil {
		return nil, err
	}
	j.Status = Status(status)
	j.CreatedAt = time.UnixMilli(created).UTC()
	if started.Valid {
		t := time.UnixMilli(started.Int64).UTC()
		j.StartedAt = &t
	}
	if finished.Valid {
		t := time.UnixMilli(finished.Int64).UTC()
		j.FinishedAt = &t
	}
	return &j, nil
}

// GetJob returns one job with its metrics, or ErrNotFound.
func (s *Store) GetJob(ctx context.Context, id int64) (*Job, error) {
	j, err := scanJob(s.db.QueryRowContext(ctx, jobSelect+` WHERE j.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return j, err
}

// ListJobs returns the most recent jobs, newest first.
func (s *Store) ListJobs(ctx context.Context, limit int) ([]Job, error) {
	rows, err := s.db.QueryContext(ctx, jobSelect+` ORDER BY j.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jobs := make([]Job, 0, limit)
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, *j)
	}
	return jobs, rows.Err()
}

// ListItems returns up to limit items of a job with ID greater than afterID,
// oldest first. Callers poll with the last ID they have seen to receive only
// new rows.
func (s *Store) ListItems(ctx context.Context, jobID, afterID int64, limit int) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, job_id, url, path, depth, status_code, title, meta_json, headings_json, links_json,
		        link_count, content_type, bytes, duration_ms, error, fetched_at
		 FROM crawl_items
		 WHERE job_id = ? AND id > ?
		 ORDER BY id ASC
		 LIMIT ?`, jobID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Item, 0, 64)
	for rows.Next() {
		var (
			it                    Item
			meta, headings, links string
			fetched               int64
		)
		if err := rows.Scan(&it.ID, &it.JobID, &it.URL, &it.Path, &it.Depth, &it.StatusCode, &it.Title,
			&meta, &headings, &links, &it.LinkCount, &it.ContentType, &it.Bytes, &it.DurationMs,
			&it.Error, &fetched); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(meta), &it.Meta); err != nil {
			return nil, fmt.Errorf("item %d: decode meta: %w", it.ID, err)
		}
		if err := json.Unmarshal([]byte(headings), &it.Headings); err != nil {
			return nil, fmt.Errorf("item %d: decode headings: %w", it.ID, err)
		}
		if err := json.Unmarshal([]byte(links), &it.Links); err != nil {
			return nil, fmt.Errorf("item %d: decode links: %w", it.ID, err)
		}
		it.FetchedAt = time.UnixMilli(fetched).UTC()
		items = append(items, it)
	}
	return items, rows.Err()
}

// GlobalStats returns metrics aggregated across all jobs.
func (s *Store) GlobalStats(ctx context.Context) (GlobalStats, error) {
	var g GlobalStats
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM crawl_jobs`).Scan(&g.TotalJobs); err != nil {
		return g, err
	}
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(CASE WHEN error = '' AND status_code < 400 THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(CASE WHEN error != '' OR status_code >= 400 THEN 1 ELSE 0 END), 0),
		        COALESCE(SUM(bytes), 0),
		        COALESCE(AVG(CASE WHEN error = '' THEN duration_ms END), 0)
		 FROM crawl_items`).Scan(&g.TotalPages, &g.FailedPages, &g.TotalBytes, &g.AvgLatencyMs)
	return g, err
}
