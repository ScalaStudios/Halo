import { DescriptionList } from "@/components/ui/description-list";
import { SectionTitle } from "@/components/ui/section-title";
import { DetailHeaderSkeleton, PageSkeleton, SkeletonText, TableSkeleton } from "@/components/ui/skeleton";

const facts = ["Membership", "Members", "Applications", "Source", "Created"];

export default function Loading() {
  return (
    <PageSkeleton
      back={{ href: "/admin/groups", label: "Groups" }}
      heading={
        <>
          <DetailHeaderSkeleton media="icon" />
          <DescriptionList
            className="grid-cols-2 rounded-lg border border-border bg-surface p-6 md:grid-cols-3 xl:grid-cols-5"
            items={facts.map((label) => ({ label, value: <SkeletonText chars={12} /> }))}
          />
        </>
      }
    >
      <section className="flex flex-col gap-6">
        <SectionTitle title={<SkeletonText chars={10} />} description={<SkeletonText chars={150} />} />
        <TableSkeleton columns={6} facets={3} rows={8} />
      </section>
    </PageSkeleton>
  );
}
