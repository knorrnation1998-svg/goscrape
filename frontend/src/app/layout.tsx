import type { Metadata } from "next";
import Link from "next/link";
import { GeistSans } from "geist/font/sans";
import { GeistMono } from "geist/font/mono";
import "./globals.css";

export const metadata: Metadata = {
  title: {
    default: "GoScrape",
    template: "%s | GoScrape",
  },
  description:
    "Start concurrent, rate-limited crawls and watch the pages come in live.",
};

function Mark() {
  return (
    <svg
      width="18"
      height="18"
      viewBox="0 0 18 18"
      fill="none"
      aria-hidden="true"
      className="text-emerald-500"
    >
      <rect x="1" y="2" width="16" height="3" rx="1" fill="currentColor" />
      <rect x="1" y="7.5" width="10" height="3" rx="1" fill="currentColor" opacity="0.7" />
      <rect x="1" y="13" width="13" height="3" rx="1" fill="currentColor" opacity="0.45" />
    </svg>
  );
}

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html
      lang="en"
      className={`${GeistSans.variable} ${GeistMono.variable} h-full antialiased`}
    >
      <body className="flex min-h-full flex-col bg-zinc-950 font-sans text-zinc-200">
        <header className="border-b border-zinc-800">
          <div className="mx-auto flex h-14 w-full max-w-6xl items-center justify-between px-4 sm:px-6">
            <Link
              href="/"
              className="flex items-center gap-2.5 rounded-sm text-[15px] font-semibold tracking-tight text-zinc-100"
            >
              <Mark />
              GoScrape
            </Link>
            <p className="hidden text-sm text-zinc-500 sm:block">
              Concurrent web crawler
            </p>
          </div>
        </header>
        {children}
      </body>
    </html>
  );
}
