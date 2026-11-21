import { Check, X } from "lucide-react";
import { ExportButton } from "@/components/settings/export-button";
import { ImportConfiguration } from "@/components/settings/import-configuration";
import { hasRole } from "@/components/settings/roles";
import { ScheduledExportForm } from "@/components/settings/settings-forms";
import { ReadOnlyNote } from "@/components/settings/shared";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { SettingsResponse } from "@/lib/settings-types";
import type { User } from "@/lib/types";

export const metadata = { title: "Export and import" };

const INCLUDED = [
  "Organization profile and every setting on these pages",
  "People, with profile fields, status and manager",
  "Groups, their rules, owners and members",
  "Administrator roles, who holds them and who is eligible to activate them",
  "Applications, with redirect URIs, scopes, token lifetimes and assigned groups",
  "Access policies, access packages and access reviews with every decision",
  "Terms of use, and who accepted which version and when",
];

const EXCLUDED = ["Passkeys, authenticator app secrets and recovery codes", "Client secrets, API keys, webhook secrets and signing keys", "Sessions, tokens and the sign-in and audit logs"];

const IMPORTED = [
  "The settings on these pages, except email templates",
  "Custom attributes, and groups with their owners and members",
  "Applications, which keep the client secrets they have",
  "Access policies, terms of use and access packages",
];

const NOT_IMPORTED = [
  "People, and who holds which administrator role",
  "Application roles and who holds them",
  "Access reviews, terms acceptances and the audit log",
  "SAML signing and encryption settings of applications",
];

export default async function ExportPage() {
  const [me, { settings, bucketConfigured }] = await Promise.all([apiGet<User>("/me"), apiGet<SettingsResponse>("/settings")]);
  const allowed = hasRole(me);
  return (
    <div className="flex max-w-4xl flex-col gap-8">
      <PageHeader
        title="Export and import"
        description="Download your directory, applications and policies as one JSON file for backups or a move to another system, and bring configuration back from one."
      />
      <Card>
        <CardHeader title="What the export contains" description="Halo streams the file as it reads the database, so large directories download without waiting." />
        <CardBody className="grid grid-cols-1 gap-8 md:grid-cols-2">
          <ExportList title="Included" items={INCLUDED} included />
          <ExportList title="Never included" items={EXCLUDED} included={false} />
        </CardBody>
      </Card>
      <div className="flex flex-col gap-4">
        <div>
          <ExportButton disabled={!allowed} />
        </div>
        {allowed ? (
          <p className="text-body-sm text-fg-3">Each download is recorded in the audit log with your name and IP address.</p>
        ) : (
          <ReadOnlyNote>Only a global administrator can export organization data.</ReadOnlyNote>
        )}
      </div>
      <Card>
        <CardHeader
          title="Import configuration"
          description="Bring this installation in line with an export from it or from another Halo. Objects are matched by id, then by name, and you see every change before anything is saved."
        />
        <CardBody className="flex flex-col gap-8">
          <div className="grid grid-cols-1 gap-8 md:grid-cols-2">
            <ExportList title="Imported" items={IMPORTED} included />
            <ExportList title="Never imported or changed" items={NOT_IMPORTED} included={false} />
          </div>
          <div className="flex flex-col gap-4">
            <ImportConfiguration canImport={allowed} />
            {allowed ? (
              <p className="text-body-sm text-fg-3">
                Each change is checked like the same change in the console and recorded in the audit log. When approvals are on, another global administrator approves the import.
              </p>
            ) : (
              <ReadOnlyNote>Only a global administrator can import configuration.</ReadOnlyNote>
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
