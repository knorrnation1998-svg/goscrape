export type TargetResult =
  | { ok: true; url: string }
  | { ok: false; message: string };

export const MAX_DEPTH = 5;

/**
 * Validates the crawl target typed by the user. A missing scheme defaults to
 * https, matching what the Go API does.
 */
export function validateTarget(raw: string): TargetResult {
  const value = raw.trim();
  if (!value) return { ok: false, message: "Enter a URL to crawl." };

  const withScheme = /^[a-z][a-z0-9+.-]*:\/\//i.test(value)
    ? value
    : `https://${value}`;

  let parsed: URL;
  try {
    parsed = new URL(withScheme);
  } catch {
    return {
      ok: false,
      message: "This is not a valid URL. Try example.com or https://example.com/docs.",
    };
  }

  if (parsed.protocol !== "http:" && parsed.protocol !== "https:") {
    return { ok: false, message: "Only http and https URLs can be crawled." };
  }

  const host = parsed.hostname;
  const looksLikeHost =
    host === "localhost" || host.includes(".") || host.startsWith("[");
  if (!looksLikeHost) {
    return {
      ok: false,
      message: `${host} has no top-level domain. Use a full host name like example.com.`,
    };
  }
  return { ok: true, url: parsed.href };
}

export function validateDepth(depth: number): boolean {
  return Number.isInteger(depth) && depth >= 0 && depth <= MAX_DEPTH;
}
