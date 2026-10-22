"use client";

import { api } from "@/lib/api";
import { usePoll } from "@/hooks/use-poll";
import { formatBytes, formatMs, formatNumber } from "@/lib/format";
import { isActive, type GlobalStats, type Job } from "@/lib/types";
import { CrawlForm } from "./crawl-form";
import { JobsTable } from "./jobs-table";
import { Stat, StatStrip } from "./stat";

const FAST_MS = 1500;
const IDLE_MS = 8000;

interface DashboardData {
  stats: GlobalStats;
  jobs: Job[];
}

async function loadDashboard(): Promise<DashboardData> {
  const [stats, jobs] = await Promise.all([api.stats(), api.listCrawls(25)]);
  return { stats, jobs };
}

// Poll quickly while any crawl is running, slowly otherwise.
function nextDelay(latest: DashboardData | null): number {
  return latest && latest.jobs.some((j) => isActive(j.status)) ? FAST_MS : IDLE_MS;
}

export function Dashboard() {
  const { data, error } = usePoll(loadDashboard, nextDelay);
  const stats = data?.stats;

  return (
    <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-8 sm:px-6">
      <div className="mb-6">
        <h1 className="text-xl font-semibold tracking-tight text-zinc-100">Crawls</h1>
        <p className="mt-1 max-w-prose text-sm text-zinc-400">
          Start a crawl, watch the workers fetch pages in real time, and export what they find.
        </p>
      </div>

      {error && (
        <div
          role="alert"
          className="mb-6 rounded-md border border-red-900/60 bg-red-950/30 px-4 py-3 text-sm text-red-300"
        >
          {error}
        </div>
      )}

      <StatStrip className="mb-6 grid-cols-2 lg:grid-cols-5">
        <Stat
          label="Pages scraped"
          value={stats ? formatNumber(stats.totalPages) : "-"}
          hint={
            stats && stats.failedPages > 0
              ? `${formatNumber(stats.failedPages)} failed`
              : undefined
          }
        />
        <Stat
          label="Active workers"
          value={stats ? formatNumber(stats.activeWorkers) : "-"}
          tone={stats && stats.activeWorkers > 0 ? "accent" : "default"}
          live={!!stats && stats.activeWorkers > 0}
          hint={
            stats
              ? `${formatNumber(stats.runningJobs)} running ${stats.runningJobs === 1 ? "crawl" : "crawls"}`
              : undefined
          }
        />
        <Stat
          label="Average latency"
          value={stats && stats.totalPages > 0 ? formatMs(stats.avgLatencyMs) : "-"}
        />
        <Stat
          label="Data transferred"
          value={stats ? formatBytes(stats.totalBytes) : "-"}
        />
        <Stat label="Crawls run" value={stats ? formatNumber(stats.totalJobs) : "-"} />
      </StatStrip>

      <section aria-labelledby="new-crawl" className="mb-8">
        <h2 id="new-crawl" className="mb-3 text-sm font-medium text-zinc-300">
          New crawl
        </h2>
        <CrawlForm />
      </section>

      <section aria-labelledby="recent-crawls">
        <h2 id="recent-crawls" className="mb-3 text-sm font-medium text-zinc-300">
          Recent crawls
        </h2>
        {data ? (
          <JobsTable jobs={data.jobs} />
        ) : (
          !error && (
            <div className="h-40 rounded-md border border-zinc-800 bg-zinc-900/30 motion-safe:animate-pulse" />
          )
        )}
      </section>
    </main>
  );
}
