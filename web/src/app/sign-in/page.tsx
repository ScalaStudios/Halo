import type { Metadata } from "next";
import { cookies } from "next/headers";
import { AuthFrame } from "@/components/auth/auth-frame";
import { ConsentStep } from "@/components/auth/consent";
import { apiPeek } from "@/lib/api/server";
import { SignInFlow } from "@/components/auth/sign-in-flow";
import type { SignInProvider } from "@/lib/federation-types";
import { authMessages } from "@/lib/i18n/auth";
import { requestLocale } from "@/lib/i18n/server";
import type { Challenge, Settings } from "@/lib/settings-types";

type Props = { searchParams: Promise<Record<string, string | string[] | undefined>> };

export async function generateMetadata({ searchParams }: Props): Promise<Metadata> {
  const messages = authMessages(await requestLocale());
  if (typeof (await searchParams).consent === "string") {
    return { title: messages.consent.title, description: messages.consent.description };
  }
  return { title: messages.title, description: messages.description };
}

export default async function SignInPage({ searchParams }: Props) {
  const params = await searchParams;
  const locale = await requestLocale();
  if (typeof params.consent === "string") {
    return (
      <AuthFrame locale={locale}>
        <ConsentStep authRequest={params.consent} />
      </AuthFrame>
    );
  }
  const authRequest = typeof params.authRequest === "string" ? params.authRequest : undefined;
  const [request, methods] = await Promise.all([
    authRequest ? apiPeek<{ application: { name: string; logoUrl: string | null; kerberosSignIn: boolean }; loginHint: string; provider?: SignInProvider }>(`/auth/request/${encodeURIComponent(authRequest)}`) : null,
    apiPeek<{ methods: string[]; emailSignIn: Settings["emailSignIn"]; challenge: Challenge | null; signUp: boolean }>("/auth/methods"),
  ]);
  return (
    <AuthFrame locale={locale}>
      <SignInFlow
        locale={locale}
        authRequest={authRequest}
        app={request?.data?.application.name ?? null}
        appLogo={request?.data?.application.logoUrl ?? null}
        expired={request?.problem?.code === "ERR_AUTH_REQUEST_EXPIRED"}
        next={typeof params.next === "string" ? params.next : undefined}
        signedIn={(await cookies()).has("halo_session")}
        allowed={(methods.data?.methods ?? ["passkey", "security-key", "totp", "recovery-codes", "magic-link"]).filter((m) => m !== "kerberos" || request?.data?.application.kerberosSignIn !== false)}
        emailSignIn={methods.data?.emailSignIn ?? "link"}
        challenge={methods.data?.challenge ?? null}
        signUp={methods.data?.signUp ?? false}
        terms={typeof params.terms === "string" ? params.terms : undefined}
        loginHint={request?.data?.loginHint}
        route={request?.data?.provider ?? null}
      />
    </AuthFrame>
  );
}
