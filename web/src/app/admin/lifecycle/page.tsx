import { History } from "lucide-react";
import { RulesTable, RunsTable } from "@/components/governance/lifecycle";
import { Card } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { SectionTitle } from "@/components/ui/section-title";
import { apiGet } from "@/lib/api/server";
import { pluralize } from "@/lib/format";
import type { LifecycleRule, LifecycleRun } from "@/lib/governance-types";
import type { Group } from "@/lib/types";

export const metadata = { title: "Lifecycle" };

export default async function LifecyclePage() {
  const [rules, runs, groups] = await Promise.all([
    apiGet<LifecycleRule[]>("/lifecycle/rules"),
    apiGet<LifecycleRun[]>("/lifecycle/runs"),
    apiGet<Group[]>("/groups"),
  ]);
  const enabled = rules.filter((r) => r.enabled).length;
  return (
    <div className="flex animate-page flex-col gap-8">
      <PageHeader
        title="Lifecycle"
        description={`${pluralize(rules.length, "rule")}, ${enabled} enabled. Every minute Halo looks for joiners, movers and leavers and runs each matching rule once per change.`}
      />
      <section className="flex flex-col gap-4">
        <SectionTitle
          title="Rules"
          description="Joiner rules run when someone is created, mover rules when their department, title or location changes, and leaver rules when they are suspended or deprovisioned."
        />
        <RulesTable rules={rules} groups={groups.filter((g) => g.kind === "assigned")} />
      </section>
      <section className="flex flex-col gap-4">
        <SectionTitle title="Run history" description="The latest 500 runs, newest first." />
        {runs.length ? (
          <RunsTable runs={runs} />
        ) : (
          <Card>
            <EmptyState icon={History} title="No runs yet" description="Runs appear here when a change matches an enabled rule." />
          </Card>
        )}
      </section>
    </div>
  );
}
