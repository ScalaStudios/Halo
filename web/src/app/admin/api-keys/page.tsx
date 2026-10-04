import Link from "next/link";
import { Bot, KeyRound } from "lucide-react";
import { ApiKeysTable } from "@/components/provisioning/api-keys-table";
import { activeKeys } from "@/components/provisioning/shared";
import { buttonClasses } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { ApiKey } from "@/lib/provisioning-types";

export const metadata = { title: "API keys" };

export default async function ApiKeysPage() {
  const keys = await apiGet<ApiKey[]>("/api-keys");
  const active = activeKeys(keys);
  const accounts = new Set(active.map((key) => key.serviceAccountId)).size;
  return (
    <div className="flex animate-page flex-col gap-8">
      <PageHeader
        title="API keys"
        description={`${pluralize(active.length, "active key")} across ${pluralize(accounts, "service account")}. Keys with the api scope call the management API; keys with the scim scope provision people over SCIM.`}
        actions={
          <Link href="/admin/service-accounts" className={buttonClasses("secondary", "sm")}>
            <Bot aria-hidden="true" size={16} strokeWidth={1.75} />
            Service accounts
          </Link>
        }
      />
      {keys.length === 0 ? (
        <Card>
          <EmptyState
            icon={KeyRound}
            title="No API keys yet"
            description="Every key belongs to a service account, acts with that account's roles and appears under its name in the audit log. Create keys from a service account's page."
            action={
              <Link href="/admin/service-accounts" className={buttonClasses("primary", "sm")}>
                Open service accounts
              </Link>
            }
          />
        </Card>
      ) : (
        <ApiKeysTable keys={keys} />
      )}
    </div>
  );
}
