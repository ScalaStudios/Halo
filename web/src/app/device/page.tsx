import type { Metadata } from "next";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { AuthFrame } from "@/components/auth/auth-frame";
import { DeviceApproval } from "@/components/infra/device-approval";

export const metadata: Metadata = {
  title: "Sign in a device",
  description: "Confirm the code shown by the Halo CLI to sign it in as you.",
};

export default async function DevicePage({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const { user_code } = await searchParams;
  const code = typeof user_code === "string" ? user_code : "";
  const here = code ? `/device?user_code=${encodeURIComponent(code)}` : "/device";
  if (!(await cookies()).has("halo_session")) redirect(`/sign-in?next=${encodeURIComponent(here)}`);
  return (
    <AuthFrame>
      <DeviceApproval initialCode={code} signIn={`/sign-in?next=${encodeURIComponent(here)}`} />
    </AuthFrame>
  );
}
