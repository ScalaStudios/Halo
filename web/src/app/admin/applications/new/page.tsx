import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { NewApplicationFlow } from "@/components/applications/new-application-flow";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { Organization } from "@/lib/types";

export const metadata = { title: "Add application" };

export default async function NewApplicationPage() {
  const org = await apiGet<Organization>("/organization");
  return (
    <div className="flex max-w-3xl animate-page flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link href="/admin/applications" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          Applications
        </Link>
        <PageHeader title="Add application" description="Register an application so people can sign in to it with Halo. You get its connection details and setup guide on the last step." />
      </div>
      <NewApplicationFlow issuer={org.issuer} />
    </div>
  );
}
