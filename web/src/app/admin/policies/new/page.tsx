import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { formOptions } from "@/components/policy/form-options";
import { PolicyForm } from "@/components/policy/policy-form";
import { PageHeader } from "@/components/ui/page-header";
import { frameMessages } from "@/lib/i18n/console/frame";
import { policyMessages } from "@/lib/i18n/console/policy";
import { requestLocale } from "@/lib/i18n/server";

export async function generateMetadata() {
  return { title: policyMessages[await requestLocale()].page.create };
}

export default async function NewPolicyPage() {
  const [options, locale] = await Promise.all([formOptions(), requestLocale()]);
  const t = policyMessages[locale].page;
  return (
    <div className="flex flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link href="/admin/policies" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          {frameMessages[locale].nav.items.policies.label}
        </Link>
        <PageHeader title={t.create} description={t.createDescription} />
      </div>
      <PolicyForm {...options} />
    </div>
  );
}
