"use client";

import Link from "next/link";
import { useState } from "react";
import { api } from "@/lib/api";
import { useCrawl } from "@/hooks/use-crawl";
import { useNow } from "@/hooks/use-now";
import { formatBytes, formatDateTime, formatDuration, formatMs, formatNumber } from "@/lib/format";
import { isActive, type CrawlItem, type Job } from "@/lib/types";
import { btnDanger, btnSecondary } from "@/lib/ui";
import { ItemsTable } from "./items-table";
import { Stat, StatStrip } from "./stat";
import { StatusBadge } from "./status-badge";

function downloadJson(job: Job, items: CrawlItem[]) {
  const payload = { exportedAt: new Date().toISOString(), job, items };
  const blob = new Blob([JSON.stringify(payload, null, 2)], { type: "application/json" });
  const href = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = href;
  link.download = `goscrape-crawl-${job.id}.json`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(href), 1000);
}

export function CrawlView({ id }: { id: number }) {
  const { job, items, error, notFound } = useCrawl(id);
  const [cancelling, setCancelling] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  const active = job ? isActive(job.status) : false;
  const now = useNow(active);

  const invalid = !Number.isInteger(id) || id < 1;
  if (invalid || notFound) {
    return (
      <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-16 text-center sm:px-6">
        <h1 className="text-lg font-semibold text-zinc-100">Crawl not found</h1>
        <p className="mt-2 text-sm text-zinc-400">
          There is no crawl with this ID. It may have been created with a different database.
        </p>
        <Link href="/" className={`${btnSecondary} mt-6`}>
          Back to crawls
        </Link>
      </main>
    );
  }

  async function cancel() {
    setCancelling(true);
    setActionError(null);
    try {
      await api.cancelCrawl(id);
    } catch (err) {
      setActionError(err instanceof Error ? err.message : "The crawl could not be cancelled.");
      setCancelling(false);
    }
  }

  let elapsed = "-";
  if (job?.startedAt) {
    const startedAt = new Date(job.startedAt).getTime();
    const end = job.finishedAt ? new Date(job.finishedAt).getTime() : now;
    if (end > 0) elapsed = formatDuration(end - startedAt);
  }

  return (
    <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-8 sm:px-6">
      <nav aria-label="Breadcrumb" className="mb-4 text-sm text-zinc-500">
        <Link href="/" className="rounded-sm hover:text-zinc-200">
          Crawls
        </Link>
        <span aria-hidden="true" className="mx-2 text-zinc-700">
          /
        </span>
        <span className="font-mono text-zinc-400">#{id}</span>
      </nav>

      {error && (
        <div
          role="alert"
          className="mb-6 rounded-md border border-red-900/60 bg-red-950/30 px-4 py-3 text-sm text-red-300"
        >
          {error}
        </div>
      )}

      {!job ? (
        !error && <div className="h-64 rounded-md border border-zinc-800 bg-zinc-900/30 motion-safe:animate-pulse" />
      ) : (
        <>
          <div className="mb-6 flex flex-wrap items-start justify-between gap-4">
            <div className="min-w-0">
              <h1 className="break-all font-mono text-lg font-medium text-zinc-100">{job.url}</h1>
              <div className="mt-2 flex flex-wrap items-center gap-x-5 gap-y-1 text-sm text-zinc-500">
                <StatusBadge status={job.status} />
                <span>
                  Depth <span className="font-mono text-zinc-300">{job.maxDepth}</span>
                </span>
                <span>
                  Created <span className="font-mono text-zinc-300">{formatDateTime(job.createdAt)}</span>
                </span>
              </div>
            </div>

            <div className="flex items-center gap-2">
              {active && (
                <button type="button" onClick={cancel} disabled={cancelling} className={btnDanger}>
                  {cancelling ? "Cancelling" : "Cancel crawl"}
                </button>
              )}
              <button
                type="button"
                onClick={() => downloadJson(job, items)}
                disabled={items.length === 0}
                className={btnSecondary}
              >
                <svg width="14" height="14" viewBox="0 0 14 14" aria-hidden="true">
                  <path
                    d="M7 1.5v8M3.5 6.5 7 10l3.5-3.5M2 12.5h10"
                    fill="none"
                    stroke="currentColor"
                    strokeWidth="1.5"
                  />
                </svg>
                Export JSON
              </button>
            </div>
          </div>

          {actionError && (
            <p role="alert" className="mb-4 text-sm text-red-400">
              {actionError}
            </p>
          )}
          {job.status === "failed" && job.error && (
            <div
              role="alert"
              className="mb-6 rounded-md border border-red-900/60 bg-red-950/30 px-4 py-3 text-sm text-red-300"
            >
              This crawl failed: <span className="font-mono">{job.error}</span>
            </div>
          )}

          <StatStrip className="mb-8 grid-cols-2 md:grid-cols-3 lg:grid-cols-6">
            <Stat label="Pages scraped" value={formatNumber(job.pagesScraped)} />
            <Stat
              label="Failed"
              value={formatNumber(job.pagesFailed)}
              tone={job.pagesFailed > 0 ? "danger" : "default"}
            />
            <Stat label="Data transferred" value={formatBytes(job.bytesTotal)} />
            <Stat
              label="Average latency"
              value={job.pagesScraped + job.pagesFailed > 0 ? formatMs(job.avgLatencyMs) : "-"}
            />
            <Stat
              label="Active workers"
              value={formatNumber(job.activeWorkers)}
              tone={job.activeWorkers > 0 ? "accent" : "default"}
              live={job.activeWorkers > 0}
            />
            <Stat label="Elapsed" value={elapsed} />
          </StatStrip>

          <ItemsTable items={items} live={active} />
        </>
      )}
    </main>
  );
}
