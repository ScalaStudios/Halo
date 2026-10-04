import "@fontsource-variable/archivo/wdth.css";
import "@fontsource-variable/inter";
import "@fontsource-variable/jetbrains-mono";
import "./globals.css";
import type { Metadata, Viewport } from "next";
import type { ReactNode } from "react";
import { Providers } from "./providers";

export const metadata: Metadata = {
  title: { default: "Halo", template: "%s · Halo" },
  description: "Halo is a free and open-source identity platform: single sign-on, passkeys, access governance and zero-trust policies you can host yourself.",
  applicationName: "Halo",
  openGraph: {
    title: "Halo",
    description: "Open-source identity, single sign-on and access governance you can host yourself.",
    siteName: "Halo",
    type: "website",
  },
};

export const viewport: Viewport = {
  themeColor: "#111111",
  colorScheme: "dark",
};

export default function RootLayout({ children }: { children: ReactNode }) {
  return (
    <html lang="en">
      <body>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
