import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { formOptions } from "@/components/policy/form-options";
import { PolicyForm } from "@/components/policy/policy-form";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { formatDateTime } from "@/lib/format";
import type { AccessPolicy } from "@/lib/policy-types";

export async function generateMetadata({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return { title: (await apiGet<AccessPolicy>(`/policies/${encodeURIComponent(id)}`)).name };
}

export default async function PolicyPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const [policy, options] = await Promise.all([apiGet<AccessPolicy>(`/policies/${encodeURIComponent(id)}`), formOptions()]);
  return (
    <div className="flex animate-page flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link href="/admin/policies" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          Policies
        </Link>
        <PageHeader title={policy.name} description={`Last changed ${formatDateTime(policy.updatedAt)}.`} />
      </div>
      <PolicyForm key={policy.updatedAt} policy={policy} {...options} />
    </div>
  );
}
