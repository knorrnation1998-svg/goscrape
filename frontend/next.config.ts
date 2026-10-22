import type { NextConfig } from "next";

// The Go server. The browser only ever talks to this app's own /api/* path,
// which Next.js proxies to the backend, so no CORS setup is needed in dev.
const API_URL = process.env.GOSCRAPE_API_URL ?? "http://localhost:8080";

const nextConfig: NextConfig = {
  cacheComponents: true,
  partialPrefetching: true,
  turbopack: {
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
  async rewrites() {
    return [
      {
        source: "/api/:path*",
        destination: `${API_URL}/api/:path*`,
      },
    ];
  },
};

export default nextConfig;
