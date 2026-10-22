"use client";

import { useEffect, useState } from "react";

/**
 * Returns the current time in ms, refreshed every second while active is true,
 * and 0 until the first tick. Reading the clock only inside timers keeps the
 * first render deterministic.
 */
export function useNow(active: boolean): number {
  const [now, setNow] = useState(0);

  useEffect(() => {
    if (!active) return;
    const tick = () => setNow(Date.now());
    const first = setTimeout(tick, 0);
    const interval = setInterval(tick, 1000);
    return () => {
      clearTimeout(first);
      clearInterval(interval);
    };
  }, [active]);

  return now;
}
