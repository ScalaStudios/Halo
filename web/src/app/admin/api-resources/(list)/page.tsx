import Link from "next/link";
import { FileKey } from "lucide-react";
import { NewResourceButton } from "@/components/infra/api-resources";
import { Tag } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { SimpleTable } from "@/components/ui/simple-table";
import { apiGet } from "@/lib/api/server";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";
import type { ApiResource } from "@/lib/infra-types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.apiResources.label };
}

export default async function ApiResourcesPage() {
  const [resources, locale] = await Promise.all([apiGet<ApiResource[]>("/api-resources"), requestLocale()]);
  const t = applicationsMessages[locale];
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.apiResources.label}
        description={t.apiResources.summary(resources.length)}
        actions={<NewResourceButton />}
      />
      {resources.length ? (
        <SimpleTable
          caption={t.apiResources.caption}
          head={[t.common.name, t.apiResources.identifier, t.apiResources.scopes, t.common.applications, t.apiResources.tokenLifetime]}
          rows={resources.map((r) => [
            <Link key="name" href={`/admin/api-resources/${r.id}`} className="flex max-w-60 min-w-40 flex-col">
              <span className="truncate font-medium text-fg underline-offset-3 hover:underline">{r.name}</span>
              <span className="truncate text-caption text-fg-3">{r.description}</span>
            </Link>,
            <span key="identifier" className="font-mono text-code-sm break-all text-fg-2">
              {r.identifier}
            </span>,
            <span key="scopes" className="flex flex-wrap gap-1">
              {r.scopes.map((s) => (
                <Tag key={s.name}>{s.name}</Tag>
              ))}
            </span>,
            <span key="apps" className="tnum whitespace-nowrap">
              {t.common.applicationCount(r.grants.length)}
            </span>,
            <span key="ttl" className="whitespace-nowrap">
              {t.duration(r.accessTokenTtl)}
            </span>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={FileKey}
            title={t.apiResources.empty}
            description={t.apiResources.emptyHint}
          />
        </Card>
      )}
    </div>
  );
}
