import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import { GrantsSection, ResourceActions } from "@/components/infra/api-resources";
import { TokenValidationCard } from "@/components/infra/token-validation";
import { Tag } from "@/components/ui/badge";
import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { apiGet } from "@/lib/api/server";
import { formatDate, formatDuration, pluralize } from "@/lib/format";
import type { ApiResource } from "@/lib/infra-types";
import type { Application, Organization } from "@/lib/types";

export async function generateMetadata({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return { title: (await apiGet<ApiResource>(`/api-resources/${encodeURIComponent(id)}`)).name };
}

export default async function ApiResourcePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const [resource, applications, org] = await Promise.all([
    apiGet<ApiResource>(`/api-resources/${encodeURIComponent(id)}`),
    apiGet<Application[]>("/applications"),
    apiGet<Organization>("/organization"),
  ]);
  const grantable = applications.filter((a) => a.protocol !== "saml").map((a) => ({ id: a.id, name: a.name }));

  return (
    <div className="flex animate-page flex-col gap-8">
      <div className="flex flex-col gap-6">
        <Link href="/admin/api-resources" className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
          <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
          API resources
        </Link>
        <header className="flex flex-col gap-6 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex min-w-0 flex-col gap-2">
            <h1 className="text-h2 text-fg">{resource.name}</h1>
            <p className="font-mono text-code-sm break-all text-fg-2">{resource.identifier}</p>
            {resource.description ? <p className="max-w-2xl text-body text-fg-3">{resource.description}</p> : null}
          </div>
          <ResourceActions resource={resource} />
        </header>
        <DescriptionList
          className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-3"
          items={[
            { label: "Access token lifetime", value: formatDuration(resource.accessTokenTtl) },
            { label: "Applications", value: pluralize(resource.grants.length, "application") },
            { label: "Created", value: formatDate(resource.createdAt) },
          ]}
        />
      </div>

      <div className="flex flex-col gap-6">
        <SectionTitle title="Scopes" description="What the API lets callers do. Tokens list the granted scopes in the scope claim, separated by spaces." />
        <SimpleTable
          caption={`Scopes of ${resource.name}`}
          head={["Scope", "Description", "Granted to"]}
          rows={resource.scopes.map((scope) => [
            <Tag key="name">{scope.name}</Tag>,
            <span key="description">{scope.description || "—"}</span>,
            <span key="granted" className="tnum whitespace-nowrap">
              {pluralize(resource.grants.filter((g) => g.scopes.includes(scope.name)).length, "application")}
            </span>,
          ])}
        />
      </div>

      <GrantsSection resource={resource} applications={grantable} />

      <TokenValidationCard issuer={org.issuer} identifier={resource.identifier} />
    </div>
  );
}
