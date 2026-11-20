import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { DetailHeaderSkeleton, PageSkeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";

const facts = ["Events", "Conditions", "Last message", "Rule ID"];

export default function Loading() {
  return (
    <PageSkeleton
      back={{ href: "/admin/notification-rules", label: "Notification rules" }}
      heading={
        <>
          <DetailHeaderSkeleton media="icon" actions={2} />
          <DescriptionList
            className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-4"
            items={facts.map((label) => ({ label, value: <SkeletonText chars={14} /> }))}
          />
        </>
      }
    >
      <section className="flex flex-col gap-6">
        <SectionTitle title="Messages" description={<SkeletonText chars={110} />} />
        <TableSkeleton simple columns={6} rows={3} />
      </section>
    </PageSkeleton>
  );
}
