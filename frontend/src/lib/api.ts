import type { CrawlDetail, GlobalStats, Job } from "./types";

export class ApiError extends Error {
  readonly status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

const UNREACHABLE =
  "Cannot reach the GoScrape API. Start the Go server (go run ./cmd/server) and try again.";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(path, { ...init, cache: "no-store" });
  } catch {
    throw new ApiError(UNREACHABLE, 0);
  }

  const body: unknown = await res.json().catch(() => null);

  if (!res.ok) {
    const message =
      body && typeof body === "object" && "error" in body
        ? String((body as { error: unknown }).error)
        : null;
    // The Next.js proxy answers 500 when the Go server is down.
    if (!message && res.status >= 500) throw new ApiError(UNREACHABLE, res.status);
    throw new ApiError(message ?? `Request failed with status ${res.status}.`, res.status);
  }
  return body as T;
}

export const api = {
  stats: () => request<GlobalStats>("/api/stats"),

  listCrawls: (limit = 50) =>
    request<{ crawls: Job[] }>(`/api/crawls?limit=${limit}`).then((r) => r.crawls),

  getCrawl: (id: number, afterId = 0, limit = 5000) =>
    request<CrawlDetail>(`/api/crawls/${id}?afterId=${afterId}&limit=${limit}`),

  startCrawl: (url: string, maxDepth: number) =>
    request<Job>("/api/crawl", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url, maxDepth }),
    }),

  cancelCrawl: (id: number) =>
    request<Job>(`/api/crawls/${id}/cancel`, { method: "POST" }),
};
