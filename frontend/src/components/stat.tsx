import type { ReactNode } from "react";

type Tone = "default" | "accent" | "danger";

const toneClass: Record<Tone, string> = {
  default: "text-zinc-100",
  accent: "text-emerald-400",
  danger: "text-red-400",
};

/** A row of metrics separated by 1px rules, sharing one outer border. */
export function StatStrip({
  children,
  className = "",
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <dl
      className={`grid gap-px overflow-hidden rounded-md border border-zinc-800 bg-zinc-800 ${className}`}
    >
      {children}
    </dl>
  );
}

export function Stat({
  label,
  value,
  hint,
  tone = "default",
  live = false,
}: {
  label: string;
  value: ReactNode;
  hint?: ReactNode;
  tone?: Tone;
  /** Shows a pulsing dot next to the value (used for active workers). */
  live?: boolean;
}) {
  return (
    <div className="bg-zinc-950 px-4 py-3.5">
      <dt className="text-sm text-zinc-500">{label}</dt>
      <dd className="mt-1.5 flex items-center gap-2">
        <span
          className={`font-mono text-2xl font-medium tabular-nums tracking-tight ${toneClass[tone]}`}
        >
          {value}
        </span>
        {live && (
          <span
            aria-hidden="true"
            className="size-2 rounded-full bg-emerald-500 motion-safe:animate-pulse"
          />
        )}
      </dd>
      {hint && <p className="mt-1 text-xs text-zinc-500">{hint}</p>}
    </div>
  );
}
