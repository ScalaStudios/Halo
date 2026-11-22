import type { Metadata } from "next";
import { AuthFrame } from "@/components/auth/auth-frame";
import { EnrollFlow, ResumeAfterCheck, type Enrollee } from "@/components/auth/enroll-flow";
import { SignUpFlow } from "@/components/auth/sign-up-flow";
import { apiPeek } from "@/lib/api/server";
import { authMessages } from "@/lib/i18n/auth";
import { requestLocale } from "@/lib/i18n/server";
import type { Challenge } from "@/lib/settings-types";

type Props = { searchParams: Promise<Record<string, string | string[] | undefined>> };

export async function generateMetadata({ searchParams }: Props): Promise<Metadata> {
  const { token, proofing } = await searchParams;
  const messages = authMessages(await requestLocale());
  const { title, description } = typeof token === "string" || proofing === "done" ? messages.enrollment : messages.registration;
  return { title, description };
}

export default async function SignUpPage({ searchParams }: Props) {
  const { token, proofing } = await searchParams;
  if (typeof token !== "string" && proofing === "done") {
    const locale = await requestLocale();
    return (
      <AuthFrame locale={locale}>
        <ResumeAfterCheck locale={locale} signUp />
      </AuthFrame>
    );
  }
  if (typeof token === "string") {
    const [{ data, problem }, locale] = await Promise.all([apiPeek<Enrollee>(`/auth/sign-up?token=${encodeURIComponent(token)}`), requestLocale()]);
    return (
      <AuthFrame locale={locale}>
        <EnrollFlow locale={locale} token={token} enrollee={data} problem={problem} signUp />
      </AuthFrame>
    );
  }
  const [methods, locale] = await Promise.all([apiPeek<{ challenge: Challenge | null; signUp: boolean }>("/auth/methods"), requestLocale()]);
  return (
    <AuthFrame locale={locale}>
      <SignUpFlow locale={locale} open={methods.data?.signUp ?? false} challenge={methods.data?.challenge ?? null} />
    </AuthFrame>
  );
}
