import Link from "next/link";
import { ChevronRight, LayoutGrid, MonitorSmartphone } from "lucide-react";
import { Card } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { PageSkeleton, Skeleton, SkeletonText } from "@/components/ui/skeleton";

const links = [
  { href: "/account/sessions", icon: MonitorSmartphone, label: "Sessions" },
  { href: "/account/applications", icon: LayoutGrid, label: "Applications" },
];

export default function Loading() {
  return (
    <PageSkeleton large title="Account" description={62}>
      <div className="flex flex-col gap-8">
        <Card className="flex flex-col gap-6 p-6">
          <div className="flex min-w-0 items-center gap-4">
            <Skeleton className="size-12 shrink-0 rounded-full" />
            <div className="flex min-w-0 flex-col gap-1">
              <p className="text-h3">
                <SkeletonText chars={12} />
              </p>
              <p className="text-body-sm">
                <SkeletonText chars={20} />
              </p>
            </div>
          </div>
          <DescriptionList columns={2} items={["Title", "Department", "Location"].map((label) => ({ label, value: <SkeletonText chars={14} /> }))} />
          <p className="border-t border-border pt-4 text-caption">
            <SkeletonText chars={90} />
          </p>
        </Card>
        <Card className="flex flex-col gap-4 p-6 sm:flex-row sm:items-center">
          <Skeleton className="size-10 shrink-0 rounded-md" />
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <p className="text-h4">
              <SkeletonText chars={44} />
            </p>
            <p className="text-body-sm">
              <SkeletonText chars={80} />
            </p>
          </div>
        </Card>
        <ul className="overflow-hidden rounded-lg border border-border bg-surface">
          {links.map((link) => {
            const Icon = link.icon;
            return (
              <li key={link.href} className="border-t border-border first:border-t-0">
                <Link href={link.href} className="flex items-center gap-4 px-6 py-4 transition-colors duration-fast ease-brand hover:bg-hover">
                  <Icon aria-hidden="true" size={20} strokeWidth={1.75} className="shrink-0 text-fg-3" />
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span className="text-body-sm font-semibold text-fg">{link.label}</span>
                    <span className="text-body-sm">
                      <SkeletonText chars={26} />
                    </span>
                  </span>
                  <ChevronRight aria-hidden="true" size={16} strokeWidth={1.75} className="shrink-0 text-fg-3" />
                </Link>
              </li>
            );
          })}
        </ul>
      </div>
    </PageSkeleton>
  );
}
