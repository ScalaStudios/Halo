import { SectionTitle } from "@/components/ui/section-title";
import { ListSkeleton, PageSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton large back={{ href: "/account/groups", label: "Groups" }} description={90}>
      <section className="flex flex-col gap-4">
        <SectionTitle title="Members" description="Members get the group's applications at their next sign-in. Changes are recorded in the audit log." />
        <ListSkeleton rows={4} className="overflow-hidden rounded-lg border border-border bg-surface" />
      </section>
    </PageSkeleton>
  );
}
