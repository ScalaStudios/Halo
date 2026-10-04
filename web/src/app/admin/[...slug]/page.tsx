import Link from "next/link";
import { notFound } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { buttonClasses } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { allItems, BASE } from "@/components/console/nav";

type Params = Promise<{ slug: string[] }>;

async function itemFor(params: Params) {
  const { slug } = await params;
  return allItems.find((item) => item.href === `${BASE}/${slug.join("/")}`);
}

export async function generateMetadata({ params }: { params: Params }) {
  return { title: (await itemFor(params))?.label ?? "Page not found" };
}

export default async function PlaceholderPage({ params }: { params: Params }) {
  const item = await itemFor(params);
  if (!item) notFound();

  return (
    <div className="flex animate-page flex-col gap-8">
      <PageHeader title={item.label} description={item.summary} />
      <Card>
        <EmptyState
          icon={item.icon}
          title="Not available yet"
          description="This section is planned but isn't built yet."
          action={
            <Link href={BASE} className={buttonClasses("secondary", "sm")}>
              <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
              Back to Overview
            </Link>
          }
        />
      </Card>
    </div>
  );
}
