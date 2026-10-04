import type { Metadata } from "next";
import { AuthFrame } from "@/components/auth/auth-frame";
import { EnrollFlow, type Enrollee } from "@/components/auth/enroll-flow";
import { apiPeek } from "@/lib/api/server";

export const metadata: Metadata = {
  title: "Set up your passkey",
  description: "Create the passkey you'll use to sign in to Halo.",
};

export default async function EnrollPage({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const { token } = await searchParams;
  const value = typeof token === "string" ? token : "";
  const { data, problem } = await apiPeek<Enrollee>(`/auth/enroll?token=${encodeURIComponent(value)}`);
  return (
    <AuthFrame>
      <EnrollFlow token={value} enrollee={data} problem={problem} />
    </AuthFrame>
  );
}
