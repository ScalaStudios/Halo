import type { Metadata } from "next";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { AuthFrame } from "@/components/auth/auth-frame";
import { DeviceApproval } from "@/components/infra/device-approval";
import { authMessages } from "@/lib/i18n/auth";
import { requestLocale } from "@/lib/i18n/server";

export async function generateMetadata(): Promise<Metadata> {
  const { title, description } = authMessages(await requestLocale()).device;
  return { title, description };
}

export default async function DevicePage({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const { user_code } = await searchParams;
  const code = typeof user_code === "string" ? user_code : "";
  const here = code ? `/device?user_code=${encodeURIComponent(code)}` : "/device";
  if (!(await cookies()).has("halo_session")) redirect(`/sign-in?next=${encodeURIComponent(here)}`);
  return (
    <AuthFrame locale={await requestLocale()}>
      <DeviceApproval initialCode={code} signIn={`/sign-in?next=${encodeURIComponent(here)}`} />
    </AuthFrame>
  );
}
