import type { Metadata } from "next";
import { AuthFrame } from "@/components/auth/auth-frame";
import { MagicLinkRedeem } from "@/components/auth/magic-link";
import { authMessages } from "@/lib/i18n/auth";
import { requestLocale } from "@/lib/i18n/server";

export async function generateMetadata(): Promise<Metadata> {
  const { title, description } = authMessages(await requestLocale()).magicLink;
  return { title, description };
}

export default async function MagicLinkPage({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const [{ token }, locale] = await Promise.all([searchParams, requestLocale()]);
  return (
    <AuthFrame locale={locale}>
      <MagicLinkRedeem locale={locale} token={typeof token === "string" ? token : ""} />
    </AuthFrame>
  );
}
