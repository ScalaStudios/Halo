import { Check, X } from "lucide-react";
import { ExportButton } from "@/components/settings/export-button";
import { ImportConfiguration } from "@/components/settings/import-configuration";
import { hasRole } from "@/components/settings/roles";
import { ScheduledExportForm } from "@/components/settings/settings-forms";
import { ReadOnlyNote } from "@/components/settings/shared";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { frameMessages } from "@/lib/i18n/console/frame";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { requestLocale } from "@/lib/i18n/server";
import type { SettingsResponse } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.export.label };
}

export default async function ExportPage() {
  const [me, { settings, bucketConfigured }, locale] = await Promise.all([apiGet<User>("/me"), apiGet<SettingsResponse>("/settings"), requestLocale()]);
  const allowed = hasRole(me);
  const t = settingsMessages[locale].exportPage;
  return (
    <div className="flex max-w-4xl flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.export.label}
        description={t.description}
      />
      <Card>
        <CardHeader title={t.contains} description={t.containsHint} />
        <CardBody className="grid grid-cols-1 gap-8 md:grid-cols-2">
          <ExportList title={t.included} items={t.includedItems} included />
          <ExportList title={t.excluded} items={t.excludedItems} included={false} />
        </CardBody>
      </Card>
      <div className="flex flex-col gap-4">
        <div>
          <ExportButton disabled={!allowed} />
        </div>
        {allowed ? (
          <p className="text-body-sm text-fg-3">{t.recorded}</p>
        ) : (
          <ReadOnlyNote>{t.exportReadOnly}</ReadOnlyNote>
        )}
      </div>
      <Card>
        <CardHeader
          title={t.import}
          description={t.importHint}
        />
        <CardBody className="flex flex-col gap-8">
          <div className="grid grid-cols-1 gap-8 md:grid-cols-2">
            <ExportList title={t.imported} items={t.importedItems} included />
            <ExportList title={t.notImported} items={t.notImportedItems} included={false} />
          </div>
          <div className="flex flex-col gap-4">
            <ImportConfiguration canImport={allowed} />
            {allowed ? (
              <p className="text-body-sm text-fg-3">{t.importNote}</p>
            ) : (
              <ReadOnlyNote>{t.importReadOnly}</ReadOnlyNote>
            )}
          </div>
        </CardBody>
      </Card>
      <ScheduledExportForm settings={settings} bucketConfigured={bucketConfigured} canEdit={allowed} />
    </div>
  );
}

function ExportList({ title, items, included }: { title: string; items: string[]; included: boolean }) {
  const Icon = included ? Check : X;
  return (
    <div className="flex flex-col gap-3">
      <h3 className="text-label text-fg-3">{title}</h3>
      <ul className="flex flex-col gap-2">
        {items.map((item) => (
          <li key={item} className="flex items-start gap-2 text-body-sm text-fg-2">
            <Icon aria-hidden="true" size={16} strokeWidth={1.75} className={included ? "mt-0.5 shrink-0 text-success" : "mt-0.5 shrink-0 text-fg-3"} />
            {item}
          </li>
        ))}
      </ul>
    </div>
  );
}
