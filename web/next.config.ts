import type { NextConfig } from "next";

const api = process.env.HALO_API_URL ?? "http://localhost:8080";
const scripts = `'self' 'unsafe-inline'${process.env.NODE_ENV === "development" ? " 'unsafe-eval'" : ""}`;
const csp = `default-src 'self'; script-src ${scripts}; style-src 'self' 'unsafe-inline'; img-src 'self' data:`;
const hcaptcha = "https://hcaptcha.com https://*.hcaptcha.com";
const signInCsp = `default-src 'self'; script-src ${scripts} https://challenges.cloudflare.com ${hcaptcha}; style-src 'self' 'unsafe-inline' ${hcaptcha}; img-src 'self' data:; frame-src https://challenges.cloudflare.com ${hcaptcha}; connect-src 'self' ${hcaptcha}`;

const config: NextConfig = {
  distDir: process.env.HALO_WEB_DIST ?? ".next",
  output: "standalone",
  reactStrictMode: true,
  poweredByHeader: false,
  devIndicators: false,
  agentRules: false,
  async headers() {
    return [
      { source: "/(.*)", headers: [{ key: "Content-Security-Policy", value: csp }] },
      { source: "/sign-in", headers: [{ key: "Content-Security-Policy", value: signInCsp }] },
    ];
  },
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
