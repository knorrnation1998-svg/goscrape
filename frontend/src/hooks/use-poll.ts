"use client";

import { useEffect, useRef, useState } from "react";

export interface PollState<T> {
  data: T | null;
  error: string | null;
}

/**
 * Calls load() immediately and then again after delayFor(latestData)
 * milliseconds, always waiting for the previous call to finish so slow
 * responses never pile up. Polling pauses while the tab is hidden.
 *
 * delayFor receives the most recent successful result (null before the first
 * one), which lets callers poll fast while something is running and slowly
 * when idle.
 */
export function usePoll<T>(
  load: () => Promise<T>,
  delayFor: (latest: T | null) => number,
): PollState<T> {
  const [state, setState] = useState<PollState<T>>({ data: null, error: null });
  const loadRef = useRef(load);
  const delayRef = useRef(delayFor);

  useEffect(() => {
    loadRef.current = load;
    delayRef.current = delayFor;
  }, [load, delayFor]);

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let latest: T | null = null;

    const run = async () => {
      if (document.visibilityState === "visible") {
        try {
          const data = await loadRef.current();
          if (cancelled) return;
          latest = data;
          setState({ data, error: null });
        } catch (err) {
          if (cancelled) return;
          const message = err instanceof Error ? err.message : "Request failed.";
          setState((prev) => ({ data: prev.data, error: message }));
        }
      }
      if (!cancelled) timer = setTimeout(run, delayRef.current(latest));
    };

    void run();
    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, []);

  return state;
}
