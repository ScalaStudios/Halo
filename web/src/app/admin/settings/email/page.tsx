import { EmailTemplateEditor } from "@/components/settings/email-templates";
import { hasRole } from "@/components/settings/roles";
import { PageHeader } from "@/components/ui/page-header";
import { TabNav } from "@/components/ui/tab-nav";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { requestLocale } from "@/lib/i18n/server";
import type { EmailTemplate } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.emailTemplates.label };
}

export default async function EmailTemplatesPage({ searchParams }: { searchParams: Promise<{ template?: string }> }) {
  const [{ template }, templates, me, locale] = await Promise.all([searchParams, apiGet<EmailTemplate[]>("/email-templates"), apiGet<User>("/me"), requestLocale()]);
  const title = frameMessages[locale].nav.items.emailTemplates.label;
  const current = templates.find((t) => t.kind === template) ?? templates[0];
  return (
    <div className="flex max-w-6xl flex-col gap-8">
      <PageHeader title={title} description={settingsMessages[locale].emailTemplates.description} />
      <div className="flex flex-col gap-6">
        <TabNav label={title} tabs={templates.map((t) => ({ id: t.kind, label: t.label }))} active={current.kind} hrefFor={(kind) => `/admin/settings/email?template=${kind}`} />
        <EmailTemplateEditor key={current.kind} template={current} canEdit={hasRole(me)} />
      </div>
    </div>
  );
}
