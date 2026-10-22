"use client";

import { useId, useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { api } from "@/lib/api";
import { MAX_DEPTH, validateDepth, validateTarget } from "@/lib/validate";
import { btnPrimary, inputBase } from "@/lib/ui";

const DEPTHS = Array.from({ length: MAX_DEPTH + 1 }, (_, i) => i);

export function CrawlForm() {
  const router = useRouter();
  const urlId = useId();
  const hintId = useId();
  const errorId = useId();

  const [url, setUrl] = useState("");
  const [depth, setDepth] = useState(1);
  const [fieldError, setFieldError] = useState<string | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  async function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending) return;

    const target = validateTarget(url);
    if (!target.ok) {
      setFieldError(target.message);
      setSubmitError(null);
      return;
    }
    if (!validateDepth(depth)) {
      setSubmitError(`Depth must be a whole number from 0 to ${MAX_DEPTH}.`);
      return;
    }

    setFieldError(null);
    setSubmitError(null);
    setPending(true);
    try {
      const job = await api.startCrawl(target.url, depth);
      router.push(`/crawls/${job.id}`);
    } catch (err) {
      setSubmitError(err instanceof Error ? err.message : "The crawl could not be started.");
      setPending(false);
    }
  }

  return (
    <form
      onSubmit={onSubmit}
      noValidate
      className="rounded-md border border-zinc-800 bg-zinc-900/40 p-4 sm:p-5"
    >
      <div className="flex flex-col gap-4 lg:flex-row lg:items-end">
        <div className="min-w-0 flex-1">
          <label htmlFor={urlId} className="mb-1.5 block text-sm font-medium text-zinc-300">
            Target URL
          </label>
          <input
            id={urlId}
            type="text"
            inputMode="url"
            autoComplete="off"
            spellCheck={false}
            placeholder="https://example.com"
            value={url}
            onChange={(e) => {
              setUrl(e.target.value);
              if (fieldError) setFieldError(null);
            }}
            aria-invalid={fieldError ? true : undefined}
            aria-describedby={fieldError ? errorId : hintId}
            className={`${inputBase} font-mono`}
          />
        </div>

        <fieldset className="shrink-0">
          <legend className="mb-1.5 text-sm font-medium text-zinc-300">Depth</legend>
          <div className="flex h-10 overflow-hidden rounded-md border border-zinc-800 bg-zinc-900">
            {DEPTHS.map((d) => (
              <label key={d} className="relative">
                <input
                  type="radio"
                  name="depth"
                  value={d}
                  checked={depth === d}
                  onChange={() => setDepth(d)}
                  className="peer sr-only"
                />
                <span className="flex h-full w-10 cursor-pointer items-center justify-center border-r border-zinc-800 font-mono text-sm text-zinc-400 transition-colors hover:bg-zinc-800 hover:text-zinc-100 peer-checked:bg-emerald-600 peer-checked:text-white peer-focus-visible:outline-2 peer-focus-visible:-outline-offset-2 peer-focus-visible:outline-emerald-400 [label:last-child_&]:border-r-0">
                  {d}
                </span>
              </label>
            ))}
          </div>
        </fieldset>

        <button type="submit" disabled={pending} className={btnPrimary}>
          {pending ? "Starting crawl" : "Start crawl"}
        </button>
      </div>

      {fieldError ? (
        <p id={errorId} role="alert" className="mt-2.5 text-sm text-red-400">
          {fieldError}
        </p>
      ) : (
        <p id={hintId} className="mt-2.5 text-sm text-zinc-500">
          Depth 0 fetches only this page. Each extra level follows links one step further,
          staying on the same site.
        </p>
      )}
      {submitError && (
        <p role="alert" className="mt-2 text-sm text-red-400">
          {submitError}
        </p>
      )}
    </form>
  );
}
