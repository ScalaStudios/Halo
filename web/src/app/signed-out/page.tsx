import type { Metadata } from "next";
import { AuthFrame } from "@/components/auth/auth-frame";
import { SignedOut } from "@/components/auth/signed-out";
import { authMessages } from "@/lib/i18n/auth";
import { requestLocale } from "@/lib/i18n/server";

export async function generateMetadata(): Promise<Metadata> {
  const { title, description } = authMessages(await requestLocale()).signedOut;
  return { title, description };
}

export default async function SignedOutPage({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const { next, confirm } = await searchParams;
  return (
    <AuthFrame locale={await requestLocale()}>
      <SignedOut next={typeof next === "string" ? next : undefined} confirm={confirm === "1"} />
    </AuthFrame>
  );
}
