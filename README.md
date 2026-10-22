# GoScrape
## 🌐 Live Demo

* **Frontend Dashboard (Vercel):**(https://goscrape.vercel.app/)
* **Backend API (Render):** (https://goscrape-hl4r.onrender.com)

A concurrent web crawler with a live dashboard. A Go backend runs a per-job worker pool, rate-limits each domain with a token bucket, stores results in SQLite and can abort a crawl mid-flight. A Next.js dashboard starts crawls and shows them live.

## Directory tree

```
goscrape/
├── README.md
├── backend/
│   ├── go.mod
│   ├── cmd/server/
│   │   ├── main.go                 # flags, wiring, graceful shutdown
│   │   └── api.go                  # REST handlers + middleware
│   └── internal/
│       ├── crawler/
│       │   ├── crawler.go          # Manager, coordinator + worker pool, cancellation
│       │   ├── limiter.go          # per-domain token bucket
│       │   ├── crawler_test.go
│       │   └── limiter_test.go
│       ├── parser/
│       │   ├── parser.go           # HTTP fetch (SSRF-safe) + goquery extraction
│       │   └── parser_test.go
│       └── storage/
│           └── storage.go          # SQLite: jobs, items, aggregates
└── frontend/                       # Next.js App Router + TypeScript + Tailwind
    ├── next.config.ts              # proxies /api/* to the Go server
    └── src/
        ├── app/                    # layout, dashboard page, crawls/[id] page
        ├── components/             # dashboard, crawl-form, jobs-table, items-table, crawl-view, ...
        ├── hooks/                  # use-poll, use-crawl, use-now
        └── lib/                    # api client, types, validation, formatting
```

## Dependencies

**Backend** (`backend/go.mod`, Go 1.23+):

| Module | Used for |
| --- | --- |
| `github.com/PuerkitoBio/goquery` | HTML parsing and extraction |
| `golang.org/x/net` (`html/charset`) | decoding non-UTF-8 pages |
| `modernc.org/sqlite` | SQLite driver, pure Go (no CGO needed) |

Everything else (HTTP server and router, `log/slog`, `context`) is the standard library.

**Frontend**: `next`, `react`, `react-dom`, `geist`, `tailwindcss` (see `frontend/package.json`).

## Setup

```bash
# 1. Backend  (first time: fetches the modules above and writes go.sum)
cd backend
go mod tidy
go test -race ./...
go run ./cmd/server            # listens on :8080, creates goscrape.db

# 2. Frontend (second terminal)
cd frontend
npm install
npm run dev                    # http://localhost:3000
```

The browser only talks to Next.js; `next.config.ts` proxies `/api/*` to the Go server. If the backend isn't on `http://localhost:8080`, set `GOSCRAPE_API_URL`.

### Backend flags / environment

| Flag | Env | Default | Meaning |
| --- | --- | --- | --- |
| `-addr` | `GOSCRAPE_ADDR` | `:8080` | listen address |
| `-db` | `GOSCRAPE_DB` | `goscrape.db` | SQLite file |
| `-workers` | | `8` | concurrent workers per crawl |
| `-max-pages` | | `500` | page cap per crawl |
| `-rate` | | `2` | requests/second per domain |
| `-burst` | | `2` | token bucket burst per domain |
| `-timeout` | | `15s` | per-request timeout |
| `-allow-private` | | `false` | allow loopback/private targets (turns SSRF protection off; use for local testing only) |
| `-cors-origin` | `GOSCRAPE_CORS_ORIGIN` | `http://localhost:3000` | origin allowed to call the API directly |

## API

| Method | Path | Purpose |
| --- | --- | --- |
| `POST` | `/api/crawl` | start a job: `{"url": "https://example.com", "maxDepth": 2}` (depth 0-5, default 1) |
| `GET` | `/api/crawls?limit=50` | recent jobs with summary metrics |
| `GET` | `/api/crawls/{id}?afterId=0&limit=5000` | job plus extracted items (`afterId` for incremental polling) |
| `POST` | `/api/crawls/{id}/cancel` | abort a running job |
| `GET` | `/api/stats` | global totals, active workers, average latency |
| `GET` | `/healthz` | liveness |

## How a crawl runs

One coordinator goroutine per job owns the BFS frontier and visited set (no locks). It hands URLs to N worker goroutines over a channel and receives results on another. Each worker waits on the per-domain token bucket, fetches, parses and stores the page. Cancelling the job context (the cancel endpoint or server shutdown) unwinds the coordinator and every worker, and the job is recorded as `cancelled`. Crawls stay on the start URL's site and skip binary and static assets.
