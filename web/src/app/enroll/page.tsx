import type { Metadata } from "next";
import { AuthFrame } from "@/components/auth/auth-frame";
import { EnrollFlow, ResumeAfterCheck, type Enrollee } from "@/components/auth/enroll-flow";
import { apiPeek } from "@/lib/api/server";
import { authMessages } from "@/lib/i18n/auth";
import { requestLocale } from "@/lib/i18n/server";

export async function generateMetadata(): Promise<Metadata> {
  const { title, description } = authMessages(await requestLocale()).enrollment;
  return { title, description };
}

export default async function EnrollPage({ searchParams }: { searchParams: Promise<Record<string, string | string[] | undefined>> }) {
  const [{ token, proofing }, locale] = await Promise.all([searchParams, requestLocale()]);
  if (typeof token !== "string" && proofing === "done") {
    return (
      <AuthFrame locale={locale}>
        <ResumeAfterCheck locale={locale} />
      </AuthFrame>
    );
  }
  const value = typeof token === "string" ? token : "";
  const { data, problem } = await apiPeek<Enrollee>(`/auth/enroll?token=${encodeURIComponent(value)}`);
  return (
    <AuthFrame locale={locale}>
      <EnrollFlow locale={locale} token={value} enrollee={data} problem={problem} />
    </AuthFrame>
  );
}
