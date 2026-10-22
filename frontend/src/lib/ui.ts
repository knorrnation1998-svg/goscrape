// Shared Tailwind class strings so controls look identical everywhere.

export const inputBase =
  "h-10 w-full rounded-md border border-zinc-800 bg-zinc-900 px-3 text-sm text-zinc-100 " +
  "placeholder:text-zinc-600 outline-none transition-colors hover:border-zinc-700 " +
  "focus-visible:border-emerald-500 focus-visible:ring-1 focus-visible:ring-emerald-500 " +
  "aria-[invalid=true]:border-red-500/70 aria-[invalid=true]:focus-visible:ring-red-500";

export const btnPrimary =
  "inline-flex h-10 shrink-0 items-center justify-center gap-2 rounded-md bg-emerald-600 px-4 " +
  "text-sm font-medium text-white transition-colors hover:bg-emerald-500 " +
  "disabled:cursor-not-allowed disabled:bg-zinc-800 disabled:text-zinc-500";

export const btnSecondary =
  "inline-flex h-9 shrink-0 items-center justify-center gap-2 rounded-md border border-zinc-800 " +
  "bg-zinc-900 px-3 text-sm font-medium text-zinc-200 transition-colors " +
  "hover:border-zinc-700 hover:bg-zinc-800 disabled:cursor-not-allowed disabled:opacity-50";

export const btnDanger =
  "inline-flex h-9 shrink-0 items-center justify-center gap-2 rounded-md border border-red-900/70 " +
  "bg-transparent px-3 text-sm font-medium text-red-300 transition-colors " +
  "hover:bg-red-950/50 disabled:cursor-not-allowed disabled:opacity-50";
