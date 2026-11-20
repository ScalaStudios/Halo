import type { Metadata } from "next";
import { AuthFrame } from "@/components/auth/auth-frame";
import { EnrollFlow, type Enrollee } from "@/components/auth/enroll-flow";
import { SignUpFlow } from "@/components/auth/sign-up-flow";
import { apiPeek } from "@/lib/api/server";
import type { Challenge } from "@/lib/settings-types";

export const metadata: Metadata = {
  title: "Sign up",
  description: "Create a Halo account with your email address and a passkey.",
};

export default async function SignUpPage({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const { token } = await searchParams;
  if (typeof token === "string") {
    const { data, problem } = await apiPeek<Enrollee>(`/auth/sign-up?token=${encodeURIComponent(token)}`);
    return (
      <AuthFrame>
        <EnrollFlow token={token} enrollee={data} problem={problem} signUp />
      </AuthFrame>
    );
  }
  const methods = await apiPeek<{ challenge: Challenge | null; signUp: boolean }>("/auth/methods");
  return (
    <AuthFrame>
      <SignUpFlow open={methods.data?.signUp ?? false} challenge={methods.data?.challenge ?? null} />
    </AuthFrame>
  );
}
