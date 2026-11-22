import { History } from "lucide-react";
import { RulesTable, RunsTable } from "@/components/governance/lifecycle";
import { Card } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import { directoryMessages } from "@/lib/i18n/console/directory";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";
import type { LifecycleRule, LifecycleRun } from "@/lib/governance-types";
import type { Attribute, Group } from "@/lib/types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.lifecycle.label };
}

export default async function LifecyclePage() {
  const [rules, runs, groups, attributes, locale] = await Promise.all([
    apiGet<LifecycleRule[]>("/lifecycle/rules"),
    apiGet<LifecycleRun[]>("/lifecycle/runs"),
    apiGet<Group[]>("/groups"),
    apiGet<Attribute[]>("/attributes"),
    requestLocale(),
  ]);
  const t = directoryMessages[locale].lifecycle;
  const enabled = rules.filter((r) => r.enabled).length;
  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.lifecycle.label}
        description={t.description(rules.length, enabled)}
      />
      <section className="flex flex-col gap-4">
        <SectionTitle title={t.rules} description={t.rulesDescription} />
        <RulesTable rules={rules} groups={groups.filter((g) => g.kind === "assigned")} attributes={attributes.filter((a) => a.type === "date")} />
      </section>
      <section className="flex flex-col gap-4">
        <SectionTitle title={t.history} description={t.historyDescription} />
        {runs.length ? (
          <RunsTable runs={runs} />
        ) : (
          <Card>
            <EmptyState icon={History} title={t.emptyTitle} description={t.emptyDescription} />
          </Card>
        )}
      </section>
    </div>
  );
}
