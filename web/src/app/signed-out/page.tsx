import type { Metadata } from "next";
import { AuthFrame } from "@/components/auth/auth-frame";
import { SignedOut } from "@/components/auth/signed-out";

export const metadata: Metadata = {
  title: "Signed out",
  description: "Sign out of Halo and return to the application you came from.",
};

export default async function SignedOutPage({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const { next } = await searchParams;
  return (
    <AuthFrame>
      <SignedOut next={typeof next === "string" ? next : undefined} />
    </AuthFrame>
  );
}
