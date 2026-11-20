import { SectionTitle } from "@/components/ui/section-title";
import { ListSkeleton, PageSkeleton } from "@/components/ui/skeleton";

const sections = [
  { title: "Your groups", description: "Assigned groups are managed by their owners. Dynamic groups follow your profile, such as your department." },
  { title: "Groups you can join", description: "Open groups add you straight away. For the others, an owner approves your request." },
];

export default function Loading() {
  return (
    <PageSkeleton
      large
      title="Groups"
      description="Groups decide which applications you see. Join the groups open to you, and manage the members of groups you own."
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
