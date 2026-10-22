"use client";

import { useEffect, useState } from "react";
import { api, ApiError } from "@/lib/api";
import { isActive, type CrawlItem, type Job } from "@/lib/types";

const PAGE_LIMIT = 5000;
const LIVE_INTERVAL_MS = 1500;
const RETRY_INTERVAL_MS = 4000;

export interface CrawlState {
  job: Job | null;
  items: CrawlItem[];
  error: string | null;
  notFound: boolean;
}

const INITIAL: CrawlState = { job: null, items: [], error: null, notFound: false };

/**
 * Loads a crawl and keeps it live. After the first response it only asks the
 * API for items newer than the last one it holds (afterId), appends them, and
 * stops polling once the job reaches a terminal state.
 */
export function useCrawl(id: number): CrawlState {
  const [state, setState] = useState<CrawlState>(INITIAL);

  useEffect(() => {
    if (!Number.isInteger(id) || id < 1) return;

    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let afterId = 0;

    const poll = async () => {
      let delay = LIVE_INTERVAL_MS;

      if (document.visibilityState === "hidden") {
        timer = setTimeout(poll, RETRY_INTERVAL_MS);
        return;
      }

      try {
        const res = await api.getCrawl(id, afterId, PAGE_LIMIT);
        if (cancelled) return;

        const fresh = res.items;
        if (fresh.length > 0) afterId = fresh[fresh.length - 1].id;
        setState((prev) => ({
          job: res.job,
          items: fresh.length > 0 ? prev.items.concat(fresh) : prev.items,
          error: null,
          notFound: false,
        }));

        if (fresh.length >= PAGE_LIMIT) {
          delay = 0; // more rows are waiting; fetch the next page right away
        } else if (!isActive(res.job.status)) {
          return; // terminal state: every item has been loaded
        }
      } catch (err) {
        if (cancelled) return;
        if (err instanceof ApiError && err.status === 404) {
          setState((prev) => ({ ...prev, notFound: true }));
          return;
        }
        const message = err instanceof Error ? err.message : "Unable to load this crawl.";
        setState((prev) => ({ ...prev, error: message }));
        delay = RETRY_INTERVAL_MS;
      }

      timer = setTimeout(poll, delay);
    };

    void poll();
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, [id]);

  return state;
}
