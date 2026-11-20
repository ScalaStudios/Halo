import { EmailTemplateEditor } from "@/components/settings/email-templates";
import { hasRole } from "@/components/settings/roles";
import { PageHeader } from "@/components/ui/page-header";
import { TabNav } from "@/components/ui/tab-nav";
import { apiGet } from "@/lib/api/server";
import type { EmailTemplate } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export const metadata = { title: "Email templates" };

export default async function EmailTemplatesPage({ searchParams }: { searchParams: Promise<{ template?: string }> }) {
  const [{ template }, templates, me] = await Promise.all([searchParams, apiGet<EmailTemplate[]>("/email-templates"), apiGet<User>("/me")]);
  const current = templates.find((t) => t.kind === template) ?? templates[0];
  return (
    <div className="flex max-w-6xl flex-col gap-8">
      <PageHeader title="Email templates" description="The subject and text of the emails Halo sends for invitations, resets, sign-in and access requests. Every email ends with Halo and your organization name." />
      <div className="flex flex-col gap-6">
        <TabNav label="Email templates" tabs={templates.map((t) => ({ id: t.kind, label: t.label }))} active={current.kind} hrefFor={(kind) => `/admin/settings/email?template=${kind}`} />
        <EmailTemplateEditor key={current.kind} template={current} canEdit={hasRole(me)} />
      </div>
    </div>
  );
}
