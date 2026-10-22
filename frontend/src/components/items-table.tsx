"use client";

import { Fragment, useId, useMemo, useState } from "react";
import type { CrawlItem } from "@/lib/types";
import { formatBytes, formatMs, formatNumber, formatTime } from "@/lib/format";
import { btnSecondary, inputBase } from "@/lib/ui";

type SortKey = "path" | "status" | "title" | "depth" | "bytes" | "latency" | "time";
type SortDir = "asc" | "desc";
type StatusClass = "2xx" | "3xx" | "4xx" | "5xx" | "error";
type StatusFilter = "all" | StatusClass;

const PAGE_SIZE = 50;
const COLUMN_COUNT = 8;

const comparators: Record<SortKey, (a: CrawlItem, b: CrawlItem) => number> = {
  path: (a, b) => a.path.localeCompare(b.path),
  status: (a, b) => a.statusCode - b.statusCode,
  title: (a, b) => a.title.localeCompare(b.title),
  depth: (a, b) => a.depth - b.depth,
  bytes: (a, b) => a.bytes - b.bytes,
  latency: (a, b) => a.durationMs - b.durationMs,
  time: (a, b) => a.id - b.id, // ids grow with insertion time
};

const descFirst: ReadonlySet<SortKey> = new Set(["bytes", "latency", "time"]);

const codeTone: Record<StatusClass, string> = {
  "2xx": "text-emerald-400",
  "3xx": "text-zinc-300",
  "4xx": "text-amber-400",
  "5xx": "text-red-400",
  error: "text-red-400",
};

const filterLabels: Record<StatusFilter, string> = {
  all: "All",
  "2xx": "2xx",
  "3xx": "3xx",
  "4xx": "4xx",
  "5xx": "5xx",
  error: "Errors",
};

function statusClass(item: CrawlItem): StatusClass {
  if (item.error) return "error";
  switch (Math.floor(item.statusCode / 100)) {
    case 2:
      return "2xx";
    case 3:
      return "3xx";
    case 4:
      return "4xx";
    case 5:
      return "5xx";
    default:
      return "error";
  }
}

function SortIcon({ dir }: { dir: SortDir | null }) {
  return (
    <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true" className="shrink-0">
      <path
        d="M5 1.5 8 4.5H2z"
        fill="currentColor"
        opacity={dir === "asc" ? 1 : 0.25}
      />
      <path
        d="M5 8.5 2 5.5h6z"
        fill="currentColor"
        opacity={dir === "desc" ? 1 : 0.25}
      />
    </svg>
  );
}

function SortTh({
  label,
  sortKey,
  sort,
  onSort,
  right = false,
  className = "",
}: {
  label: string;
  sortKey: SortKey;
  sort: { key: SortKey; dir: SortDir };
  onSort: (key: SortKey) => void;
  right?: boolean;
  className?: string;
}) {
  const active = sort.key === sortKey;
  return (
    <th
      scope="col"
      aria-sort={active ? (sort.dir === "asc" ? "ascending" : "descending") : "none"}
      className={`px-3 py-2 font-medium ${right ? "text-right" : ""} ${className}`}
    >
      <button
        type="button"
        onClick={() => onSort(sortKey)}
        className={`inline-flex items-center gap-1.5 rounded-sm transition-colors hover:text-zinc-100 ${
          active ? "text-zinc-100" : ""
        } ${right ? "flex-row-reverse" : ""}`}
      >
        {label}
        <SortIcon dir={active ? sort.dir : null} />
      </button>
    </th>
  );
}

function Chevron({ open }: { open: boolean }) {
  return (
    <svg
      width="12"
      height="12"
      viewBox="0 0 12 12"
      aria-hidden="true"
      className={`transition-transform ${open ? "rotate-90" : ""}`}
    >
      <path d="M4 2.5 8 6l-4 3.5" fill="none" stroke="currentColor" strokeWidth="1.5" />
    </svg>
  );
}

function ItemDetail({ item }: { item: CrawlItem }) {
  const meta = Object.entries(item.meta ?? {});
  const shownLinks = item.links.slice(0, 20);

  return (
    <div className="grid gap-x-8 gap-y-5 px-4 py-4 text-sm md:grid-cols-2">
      <div className="md:col-span-2">
        <a
          href={item.url}
          target="_blank"
          rel="noopener noreferrer"
          className="break-all font-mono text-[13px] text-emerald-400 underline-offset-2 hover:underline"
        >
          {item.url}
        </a>
        <p className="mt-1 font-mono text-xs text-zinc-500">
          {item.contentType || "unknown content type"}
        </p>
        {item.error && (
          <p className="mt-2 text-sm text-red-400">Request failed: {item.error}</p>
        )}
      </div>

      <section>
        <h4 className="mb-2 text-xs font-medium text-zinc-400">
          Meta tags ({formatNumber(meta.length)})
        </h4>
        {meta.length === 0 ? (
          <p className="text-zinc-500">None found.</p>
        ) : (
          <dl className="max-h-56 space-y-1.5 overflow-y-auto pr-2">
            {meta.map(([key, value]) => (
              <div key={key} className="grid grid-cols-[8rem_1fr] gap-3">
                <dt className="truncate font-mono text-xs text-zinc-500" title={key}>
                  {key}
                </dt>
                <dd className="break-words text-zinc-300">{value}</dd>
              </div>
            ))}
          </dl>
        )}
      </section>

      <section>
        <h4 className="mb-2 text-xs font-medium text-zinc-400">
          Headings ({formatNumber(item.headings.length)})
        </h4>
        {item.headings.length === 0 ? (
          <p className="text-zinc-500">None found.</p>
        ) : (
          <ul className="max-h-56 space-y-1 overflow-y-auto pr-2">
            {item.headings.map((h, i) => (
              <li
                key={i}
                style={{ paddingLeft: `${(h.level - 1) * 14}px` }}
                className="flex gap-2 text-zinc-300"
              >
                <span className="w-5 shrink-0 font-mono text-xs leading-5 text-zinc-500">
                  h{h.level}
                </span>
                <span className="break-words">{h.text}</span>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section className="md:col-span-2">
        <h4 className="mb-2 text-xs font-medium text-zinc-400">
          Links ({formatNumber(item.linkCount)})
        </h4>
        {shownLinks.length === 0 ? (
          <p className="text-zinc-500">None found.</p>
        ) : (
          <>
            <ul className="space-y-1">
              {shownLinks.map((link) => (
                <li key={link} className="truncate font-mono text-xs text-zinc-400" title={link}>
                  {link}
                </li>
              ))}
            </ul>
            {item.linkCount > shownLinks.length && (
              <p className="mt-2 text-xs text-zinc-500">
                Showing {shownLinks.length} of {formatNumber(item.linkCount)}. The JSON export
                includes up to {formatNumber(item.links.length)} per page.
              </p>
            )}
          </>
        )}
      </section>
    </div>
  );
}

export function ItemsTable({ items, live }: { items: CrawlItem[]; live: boolean }) {
  const searchId = useId();
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<StatusFilter>("all");
  const [sort, setSort] = useState<{ key: SortKey; dir: SortDir }>({ key: "time", dir: "desc" });
  const [page, setPage] = useState(0);
  const [expanded, setExpanded] = useState<ReadonlySet<number>>(new Set());

  const counts = useMemo(() => {
    const c: Record<StatusFilter, number> = { all: items.length, "2xx": 0, "3xx": 0, "4xx": 0, "5xx": 0, error: 0 };
    for (const item of items) c[statusClass(item)]++;
    return c;
  }, [items]);

  const rows = useMemo(() => {
    const needle = query.trim().toLowerCase();
    const filtered = items.filter((item) => {
      if (filter !== "all" && statusClass(item) !== filter) return false;
      if (!needle) return true;
      return (
        item.path.toLowerCase().includes(needle) ||
        item.title.toLowerCase().includes(needle) ||
        item.url.toLowerCase().includes(needle)
      );
    });
    const compare = comparators[sort.key];
    const sign = sort.dir === "asc" ? 1 : -1;
    return filtered.sort((a, b) => sign * compare(a, b));
  }, [items, query, filter, sort]);

  const pageCount = Math.max(1, Math.ceil(rows.length / PAGE_SIZE));
  const current = Math.min(page, pageCount - 1);
  const start = current * PAGE_SIZE;
  const visible = rows.slice(start, start + PAGE_SIZE);

  function onSort(key: SortKey) {
    setSort((prev) =>
      prev.key === key
        ? { key, dir: prev.dir === "asc" ? "desc" : "asc" }
        : { key, dir: descFirst.has(key) ? "desc" : "asc" },
    );
    setPage(0);
  }

  function toggle(id: number) {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function clearFilters() {
    setQuery("");
    setFilter("all");
    setPage(0);
  }

  const filtersActive = query.trim() !== "" || filter !== "all";
  const visibleFilters = (Object.keys(filterLabels) as StatusFilter[]).filter(
    (f) => f === "all" || counts[f] > 0 || f === filter,
  );

  return (
    <section aria-labelledby="pages-heading">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <h2 id="pages-heading" className="text-sm font-medium text-zinc-300">
            Scraped pages
          </h2>
          {live && (
            <span className="inline-flex items-center gap-2 text-xs text-emerald-400">
              <span
                aria-hidden="true"
                className="size-1.5 rounded-full bg-emerald-500 motion-safe:animate-pulse"
              />
              Updating live
            </span>
          )}
        </div>

        <div className="flex flex-wrap items-center gap-3">
          <div role="group" aria-label="Filter by status" className="flex overflow-hidden rounded-md border border-zinc-800">
            {visibleFilters.map((f) => (
              <button
                key={f}
                type="button"
                aria-pressed={filter === f}
                onClick={() => {
                  setFilter(f);
                  setPage(0);
                }}
                className={`flex h-9 items-center gap-1.5 border-r border-zinc-800 px-3 text-sm transition-colors last:border-r-0 ${
                  filter === f
                    ? "bg-zinc-800 text-zinc-100"
                    : "bg-zinc-900 text-zinc-400 hover:bg-zinc-800/70 hover:text-zinc-200"
                }`}
              >
                {filterLabels[f]}
                <span className="font-mono text-xs tabular-nums text-zinc-500">
                  {formatNumber(counts[f])}
                </span>
              </button>
            ))}
          </div>

          <div>
            <label htmlFor={searchId} className="sr-only">
              Search pages by path, URL or title
            </label>
            <input
              id={searchId}
              type="search"
              value={query}
              onChange={(e) => {
                setQuery(e.target.value);
                setPage(0);
              }}
              placeholder="Search path or title"
              autoComplete="off"
              spellCheck={false}
              className={`${inputBase} h-9 w-56`}
            />
          </div>
        </div>
      </div>

      <div className="overflow-x-auto rounded-md border border-zinc-800">
        <table className="w-full min-w-[820px] border-collapse text-left text-sm">
          <caption className="sr-only">Pages scraped by this crawl</caption>
          <thead className="border-b border-zinc-800 bg-zinc-900/60 text-xs text-zinc-400">
            <tr>
              <th scope="col" className="w-9 px-2 py-2">
                <span className="sr-only">Details</span>
              </th>
              <SortTh label="Path" sortKey="path" sort={sort} onSort={onSort} />
              <SortTh label="Status" sortKey="status" sort={sort} onSort={onSort} />
              <SortTh label="Title" sortKey="title" sort={sort} onSort={onSort} />
              <SortTh label="Depth" sortKey="depth" sort={sort} onSort={onSort} right />
              <SortTh label="Size" sortKey="bytes" sort={sort} onSort={onSort} right />
              <SortTh label="Latency" sortKey="latency" sort={sort} onSort={onSort} right />
              <SortTh label="Fetched" sortKey="time" sort={sort} onSort={onSort} right />
            </tr>
          </thead>
          <tbody>
            {visible.length === 0 ? (
              <tr>
                <td colSpan={COLUMN_COUNT} className="px-4 py-12 text-center">
                  {items.length === 0 ? (
                    <>
                      <p className="text-sm font-medium text-zinc-300">
                        {live ? "Waiting for the first page" : "No pages were scraped"}
                      </p>
                      <p className="mt-1 text-sm text-zinc-500">
                        {live
                          ? "Rows appear here as workers finish fetching."
                          : "The start URL did not return any data. Check the error above."}
                      </p>
                    </>
                  ) : (
                    <>
                      <p className="text-sm font-medium text-zinc-300">No pages match these filters</p>
                      {filtersActive && (
                        <button type="button" onClick={clearFilters} className={`${btnSecondary} mt-3`}>
                          Clear filters
                        </button>
                      )}
                    </>
                  )}
                </td>
              </tr>
            ) : (
              visible.map((item) => {
                const cls = statusClass(item);
                const open = expanded.has(item.id);
                return (
                  <Fragment key={item.id}>
                    <tr className="border-b border-zinc-800/70 hover:bg-zinc-900/70">
                      <td className="px-2 py-2">
                        <button
                          type="button"
                          onClick={() => toggle(item.id)}
                          aria-expanded={open}
                          aria-label={`${open ? "Hide" : "Show"} details for ${item.path}`}
                          className="flex size-6 items-center justify-center rounded-sm text-zinc-500 transition-colors hover:bg-zinc-800 hover:text-zinc-200"
                        >
                          <Chevron open={open} />
                        </button>
                      </td>
                      <td className="max-w-[20rem] px-3 py-2">
                        <span className="block truncate font-mono text-[13px] text-zinc-100" title={item.url}>
                          {item.path}
                        </span>
                      </td>
                      <td className={`px-3 py-2 font-mono text-[13px] tabular-nums ${codeTone[cls]}`}>
                        {item.error ? "err" : item.statusCode}
                      </td>
                      <td className="max-w-[18rem] px-3 py-2">
                        <span
                          className={`block truncate ${item.title ? "text-zinc-300" : "text-zinc-600"}`}
                          title={item.title || undefined}
                        >
                          {item.title || (item.error ? "Request failed" : "No title")}
                        </span>
                      </td>
                      <td className="px-3 py-2 text-right font-mono text-[13px] tabular-nums text-zinc-400">
                        {item.depth}
                      </td>
                      <td className="px-3 py-2 text-right font-mono text-[13px] tabular-nums text-zinc-400">
                        {formatBytes(item.bytes)}
                      </td>
                      <td className="px-3 py-2 text-right font-mono text-[13px] tabular-nums text-zinc-400">
                        {formatMs(item.durationMs)}
                      </td>
                      <td className="px-3 py-2 text-right font-mono text-[13px] tabular-nums text-zinc-500">
                        {formatTime(item.fetchedAt)}
                      </td>
                    </tr>
                    {open && (
                      <tr className="border-b border-zinc-800/70 bg-zinc-900/40">
                        <td colSpan={COLUMN_COUNT}>
                          <ItemDetail item={item} />
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })
            )}
          </tbody>
        </table>
      </div>

      {rows.length > 0 && (
        <div className="mt-3 flex items-center justify-between gap-3">
          <p className="font-mono text-xs tabular-nums text-zinc-500">
            {formatNumber(start + 1)}-{formatNumber(start + visible.length)} of {formatNumber(rows.length)}
          </p>
          <div className="flex items-center gap-2">
            <button
              type="button"
              className={btnSecondary}
              disabled={current === 0}
              onClick={() => setPage(current - 1)}
            >
              Previous
            </button>
            <button
              type="button"
              className={btnSecondary}
              disabled={current >= pageCount - 1}
              onClick={() => setPage(current + 1)}
            >
              Next
            </button>
          </div>
        </div>
      )}
    </section>
  );
}
