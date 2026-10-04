import Link from "next/link";
import { FileKey } from "lucide-react";
import { NewResourceButton } from "@/components/infra/api-resources";
import { Tag } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { SimpleTable } from "@/components/ui/simple-table";
import { apiGet } from "@/lib/api/server";
import { formatDuration, pluralize } from "@/lib/format";
import type { ApiResource } from "@/lib/infra-types";

export const metadata = { title: "API resources" };

export default async function ApiResourcesPage() {
  const resources = await apiGet<ApiResource[]>("/api-resources");
  return (
    <div className="flex animate-page flex-col gap-8">
      <PageHeader
        title="API resources"
        description={`${pluralize(resources.length, "API")} protected by Halo. Applications granted an API's scopes receive JWT access tokens addressed to it, which the API validates against Halo's public keys.`}
        actions={<NewResourceButton />}
      />
      {resources.length ? (
        <SimpleTable
          caption="APIs"
          head={["Name", "Identifier", "Scopes", "Applications", "Token lifetime"]}
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
              {pluralize(r.grants.length, "application")}
            </span>,
            <span key="ttl" className="whitespace-nowrap">
              {formatDuration(r.accessTokenTtl)}
            </span>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={FileKey}
            title="No APIs yet"
            description="Add an API you run, such as an internal billing service, with the scopes it understands. Then grant those scopes to the applications that call it."
          />
        </Card>
      )}
    </div>
  );
}
