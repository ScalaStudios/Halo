import { SectionTitle } from "@/components/ui/section-title";
import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Lifecycle" description={120}>
      <section className="flex flex-col gap-4">
        <SectionTitle
          title="Rules"
          description="Joiner rules run when someone is created, mover rules when their department, title or location changes, and leaver rules when they are suspended or deprovisioned."
        />
        <TableSkeleton columns={8} facets={2} rows={2} />
      </section>
      <section className="flex flex-col gap-4">
        <SectionTitle title="Run history" description="The latest 500 runs, newest first." />
        <TableSkeleton columns={7} facets={3} rows={3} />
      </section>
    </PageSkeleton>
  );
}
