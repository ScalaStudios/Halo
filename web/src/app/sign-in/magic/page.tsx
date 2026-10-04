import type { Metadata } from "next";
import { AuthFrame } from "@/components/auth/auth-frame";
import { MagicLinkRedeem } from "@/components/auth/magic-link";

export const metadata: Metadata = {
  title: "Signing in",
  description: "Finish signing in to Halo with the link from your email.",
};

export default async function MagicLinkPage({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const { token } = await searchParams;
  return (
    <AuthFrame>
      <MagicLinkRedeem token={typeof token === "string" ? token : ""} />
    </AuthFrame>
  );
}
