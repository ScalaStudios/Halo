import type { Metadata } from "next";
import { cookies } from "next/headers";
import { AuthFrame } from "@/components/auth/auth-frame";
import { apiPeek } from "@/lib/api/server";
import { SignInFlow } from "@/components/auth/sign-in-flow";

export const metadata: Metadata = {
  title: "Sign in",
  description: "Sign in to Halo with a passkey, security key, authenticator app or recovery code.",
};

export default async function SignInPage({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const params = await searchParams;
  const authRequest = typeof params.authRequest === "string" ? params.authRequest : undefined;
  const [request, methods] = await Promise.all([
    authRequest ? apiPeek<{ application: { name: string } }>(`/auth/request/${encodeURIComponent(authRequest)}`) : null,
    apiPeek<{ methods: string[] }>("/auth/methods"),
  ]);
  return (
    <AuthFrame>
      <SignInFlow
        authRequest={authRequest}
        app={request?.data?.application.name ?? null}
        expired={request?.problem?.code === "ERR_AUTH_REQUEST_EXPIRED"}
        next={typeof params.next === "string" ? params.next : undefined}
        signedIn={(await cookies()).has("halo_session")}
        allowed={methods.data?.methods ?? ["passkey", "security-key", "totp", "recovery-codes", "magic-link"]}
      />
    </AuthFrame>
  );
}
