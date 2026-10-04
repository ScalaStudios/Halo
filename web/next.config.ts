import type { NextConfig } from "next";

const api = process.env.HALO_API_URL ?? "http://localhost:8080";

const config: NextConfig = {
  distDir: process.env.HALO_WEB_DIST ?? ".next",
  output: "standalone",
  reactStrictMode: true,
  poweredByHeader: false,
  devIndicators: false,
  agentRules: false,
  async rewrites() {
    return [
      { source: "/api/:path*", destination: `${api}/api/:path*` },
      { source: "/oauth2/:path*", destination: `${api}/oauth2/:path*` },
      { source: "/.well-known/:path*", destination: `${api}/.well-known/:path*` },
      { source: "/saml/:path*", destination: `${api}/saml/:path*` },
      { source: "/scim/:path*", destination: `${api}/scim/:path*` },
    ];
  },
};

export default config;
