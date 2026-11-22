import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { formOptions } from "@/components/policy/form-options";
import { PolicyForm } from "@/components/policy/policy-form";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { formatDateTime } from "@/lib/format";
import { frameMessages } from "@/lib/i18n/console/frame";
import { policyMessages } from "@/lib/i18n/console/policy";
import { requestLocale } from "@/lib/i18n/server";
import type { AccessPolicy } from "@/lib/policy-types";

export async function generateMetadata({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return { title: (await apiGet<AccessPolicy>(`/policies/${encodeURIComponent(id)}`)).name };
}

export default async function PolicyPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const [policy, options, locale] = await Promise.all([apiGet<AccessPolicy>(`/policies/${encodeURIComponent(id)}`), formOptions(), requestLocale()]);
  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link href="/admin/policies" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          {frameMessages[locale].nav.items.policies.label}
        </Link>
        <PageHeader title={policy.name} description={policyMessages[locale].page.lastChanged(formatDateTime(policy.updatedAt, locale))} />
      </div>
      <PolicyForm key={policy.updatedAt} policy={policy} {...options} />
    </div>
  );
}
