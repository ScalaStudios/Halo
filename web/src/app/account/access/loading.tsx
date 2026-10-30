import { SectionTitle } from "@/components/ui/section-title";
import { ListSkeleton, PageSkeleton } from "@/components/ui/skeleton";

const sections = [
  { title: "Your requests", description: "You get an email when an approver decides." },
  { title: "Access you were granted", description: "Approved packages. Halo removes the access when it ends." },
  { title: "Your groups", description: "Assigned groups are managed by their owners. Dynamic groups follow your profile, such as your department." },
];

export default function Loading() {
  return (
    <PageSkeleton
      large
      title="Access"
      description="What you can reach and why. Groups decide which applications you see; roles decide what you can change in Halo."
    >
      {sections.map((section) => (
        <section key={section.title} className="flex flex-col gap-4">
          <SectionTitle title={section.title} description={section.description} />
          <ListSkeleton rows={2} className="overflow-hidden rounded-lg border border-border bg-surface" />
        </section>
      ))}
    </PageSkeleton>
  );
}
