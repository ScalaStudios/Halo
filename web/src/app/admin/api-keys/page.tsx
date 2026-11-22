import Link from "next/link";
import { Bot, KeyRound } from "lucide-react";
import { ApiKeysTable } from "@/components/provisioning/api-keys-table";
import { activeKeys } from "@/components/provisioning/shared";
import { buttonClasses } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";
import type { ApiKey } from "@/lib/provisioning-types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.apiKeys.label };
}

export default async function ApiKeysPage() {
  const [keys, locale] = await Promise.all([apiGet<ApiKey[]>("/api-keys"), requestLocale()]);
  const t = applicationsMessages[locale].apiKeys;
  const nav = frameMessages[locale].nav.items;
  const active = activeKeys(keys);
  const accounts = new Set(active.map((key) => key.serviceAccountId)).size;
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={nav.apiKeys.label}
        description={t.summary(active.length, accounts)}
        actions={
          <Link href="/admin/service-accounts" className={buttonClasses("secondary", "sm")}>
            <Bot aria-hidden="true" size={16} strokeWidth={1.75} />
            {nav.serviceAccounts.label}
          </Link>
        }
      />
      {keys.length === 0 ? (
        <Card>
          <EmptyState
            icon={KeyRound}
            title={t.empty}
            description={t.emptyHint}
            action={
              <Link href="/admin/service-accounts" className={buttonClasses("primary", "sm")}>
                {t.open}
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
