import type { JobStatus } from "@/lib/types";

const styles: Record<JobStatus, { label: string; dot: string; text: string }> = {
  pending: { label: "Pending", dot: "bg-zinc-500", text: "text-zinc-400" },
  running: { label: "Running", dot: "bg-emerald-500", text: "text-emerald-400" },
  completed: { label: "Completed", dot: "bg-emerald-500", text: "text-zinc-200" },
  failed: { label: "Failed", dot: "bg-red-500", text: "text-red-400" },
  cancelled: { label: "Cancelled", dot: "bg-amber-500", text: "text-amber-400" },
};

export function StatusBadge({ status }: { status: JobStatus }) {
  const s = styles[status];
  return (
    <span className={`inline-flex items-center gap-2 text-sm ${s.text}`}>
      <span
        aria-hidden="true"
        className={`size-2 rounded-full ${s.dot} ${
          status === "running" ? "motion-safe:animate-pulse" : ""
        }`}
      />
      {s.label}
    </span>
  );
}
