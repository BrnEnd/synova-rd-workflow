import type { NextConfig } from "next";
import path from "node:path";

const nextConfig: NextConfig = {
  output: "standalone",
  outputFileTracingRoot: path.join(__dirname),
  async rewrites() {
    const apiURL = process.env.ADMIN_API_INTERNAL_URL || "http://localhost:8080";
    return [
      {
        source: "/admin/:path*",
        destination: `${apiURL}/admin/:path*`,
      },
    ];
  },
};

export default nextConfig;
