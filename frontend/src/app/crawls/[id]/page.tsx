import type { Metadata } from "next";
import { Suspense } from "react";
import { CrawlView } from "@/components/crawl-view";

export const metadata: Metadata = {
  title: "Crawl details",
};

function Skeleton() {
  return (
    <main className="mx-auto w-full max-w-6xl flex-1 px-4 py-8 sm:px-6">
      <div className="h-64 rounded-md border border-zinc-800 bg-zinc-900/30 motion-safe:animate-pulse" />
    </main>
  );
}

// With cacheComponents the page stays synchronous and resolves the dynamic
// params inside a Suspense boundary, so the shell can be prerendered.
export default function CrawlPage({ params }: { params: Promise<{ id: string }> }) {
  return (
    <Suspense fallback={<Skeleton />}>
      {params.then(({ id }) => (
        <CrawlView id={Number(id)} />
      ))}
    </Suspense>
  );
}
