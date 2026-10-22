import Link from "next/link";
import type { Job } from "@/lib/types";
import { formatBytes, formatDateTime, formatMs, formatNumber } from "@/lib/format";
import { StatusBadge } from "./status-badge";

export function JobsTable({ jobs }: { jobs: Job[] }) {
  if (jobs.length === 0) {
    return (
      <div className="rounded-md border border-dashed border-zinc-800 px-6 py-12 text-center">
        <p className="text-sm font-medium text-zinc-300">No crawls yet</p>
        <p className="mt-1 text-sm text-zinc-500">
          Enter a URL above and start a crawl. It will show up here while it runs.
        </p>
      </div>
    );
  }

  return (
    <div className="overflow-x-auto rounded-md border border-zinc-800">
      <table className="w-full min-w-[640px] border-collapse text-left text-sm">
        <caption className="sr-only">Recent crawls</caption>
        <thead className="border-b border-zinc-800 bg-zinc-900/60 text-xs text-zinc-400">
          <tr>
            <th scope="col" className="px-3 py-2 font-medium">Target</th>
            <th scope="col" className="px-3 py-2 font-medium">Status</th>
            <th scope="col" className="px-3 py-2 text-right font-medium">Depth</th>
            <th scope="col" className="px-3 py-2 text-right font-medium">Pages</th>
            <th scope="col" className="hidden px-3 py-2 text-right font-medium md:table-cell">Failed</th>
            <th scope="col" className="hidden px-3 py-2 text-right font-medium md:table-cell">Data</th>
            <th scope="col" className="hidden px-3 py-2 text-right font-medium lg:table-cell">Avg latency</th>
            <th scope="col" className="px-3 py-2 text-right font-medium">Created</th>
          </tr>
        </thead>
        <tbody>
          {jobs.map((job) => (
            <tr
              key={job.id}
              className="relative border-b border-zinc-800/70 last:border-b-0 hover:bg-zinc-900"
            >
              <td className="max-w-[22rem] px-3 py-2.5">
                <Link
                  href={`/crawls/${job.id}`}
                  className="block truncate font-mono text-[13px] text-zinc-100 after:absolute after:inset-0 hover:text-emerald-400"
                >
                  {job.url}
                </Link>
              </td>
              <td className="px-3 py-2.5">
                <StatusBadge status={job.status} />
              </td>
              <td className="px-3 py-2.5 text-right font-mono tabular-nums text-zinc-400">
                {job.maxDepth}
              </td>
              <td className="px-3 py-2.5 text-right font-mono tabular-nums text-zinc-100">
                {formatNumber(job.pagesScraped)}
              </td>
              <td
                className={`hidden px-3 py-2.5 text-right font-mono tabular-nums md:table-cell ${
                  job.pagesFailed > 0 ? "text-red-400" : "text-zinc-600"
                }`}
              >
                {formatNumber(job.pagesFailed)}
              </td>
              <td className="hidden px-3 py-2.5 text-right font-mono tabular-nums text-zinc-400 md:table-cell">
                {formatBytes(job.bytesTotal)}
              </td>
              <td className="hidden px-3 py-2.5 text-right font-mono tabular-nums text-zinc-400 lg:table-cell">
                {job.pagesScraped + job.pagesFailed > 0 ? formatMs(job.avgLatencyMs) : "-"}
              </td>
              <td className="px-3 py-2.5 text-right font-mono tabular-nums text-zinc-500">
                {formatDateTime(job.createdAt)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
