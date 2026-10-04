import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { formOptions } from "@/components/policy/form-options";
import { PolicyForm } from "@/components/policy/policy-form";
import { PageHeader } from "@/components/ui/page-header";

export const metadata = { title: "Create policy" };

export default async function NewPolicyPage() {
  const options = await formOptions();
  return (
    <div className="flex animate-page flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link href="/admin/policies" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          Policies
        </Link>
        <PageHeader title="Create policy" description="New policies start in report-only mode, so you can see what they would do before they affect anyone." />
      </div>
      <PolicyForm {...options} />
    </div>
  );
}
