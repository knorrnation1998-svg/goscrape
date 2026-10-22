// Mirrors the JSON produced by the Go API (internal/storage and cmd/server).

export type JobStatus =
  | "pending"
  | "running"
  | "completed"
  | "failed"
  | "cancelled";

export interface Job {
  id: number;
  url: string;
  maxDepth: number;
  status: JobStatus;
  error?: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
  pagesScraped: number;
  pagesFailed: number;
  bytesTotal: number;
  avgLatencyMs: number;
  activeWorkers: number;
}

export interface Heading {
  level: number;
  text: string;
}

export interface CrawlItem {
  id: number;
  jobId: number;
  url: string;
  path: string;
  depth: number;
  statusCode: number;
  title: string;
  meta: Record<string, string>;
  headings: Heading[];
  links: string[];
  linkCount: number;
  contentType: string;
  bytes: number;
  durationMs: number;
  error?: string;
  fetchedAt: string;
}

export interface GlobalStats {
  totalJobs: number;
  totalPages: number;
  failedPages: number;
  totalBytes: number;
  avgLatencyMs: number;
  activeWorkers: number;
  runningJobs: number;
}

export interface CrawlDetail {
  job: Job;
  items: CrawlItem[];
}

export function isActive(status: JobStatus): boolean {
  return status === "pending" || status === "running";
}
