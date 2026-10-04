import { Check, X } from "lucide-react";
import { ExportButton } from "@/components/settings/export-button";
import { hasRole } from "@/components/settings/roles";
import { ReadOnlyNote } from "@/components/settings/shared";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import type { User } from "@/lib/types";

export const metadata = { title: "Data export" };

const INCLUDED = [
  "Organization profile and every setting on these pages",
  "People, with profile fields, status and manager",
  "Groups, their rules and their members",
  "Administrator roles and who holds them",
  "Applications, with redirect URIs, scopes, token lifetimes and assigned groups",
  "Access policies, access packages and access reviews with every decision",
];

const EXCLUDED = ["Passkeys, authenticator app secrets and recovery codes", "Client secrets, API keys, webhook secrets and signing keys", "Sessions, tokens and the sign-in and audit logs"];

export default async function ExportPage() {
  const me = await apiGet<User>("/me");
  const allowed = hasRole(me);
  return (
    <div className="flex max-w-4xl animate-page flex-col gap-8">
      <PageHeader title="Data export" description="Download your directory, applications and policies as one JSON file, for backups or a move to another system." />
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
