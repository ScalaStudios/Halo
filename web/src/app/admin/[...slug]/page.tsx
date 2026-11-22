import Link from "next/link";
import { notFound } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { buttonClasses } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { EmptyState } from "@/components/ui/empty-state";
import { PageHeader } from "@/components/ui/page-header";
import { allItems, BASE } from "@/components/console/nav";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

type Params = Promise<{ slug: string[] }>;

async function itemFor(params: Params) {
  const [{ slug }, locale] = await Promise.all([params, requestLocale()]);
  return allItems(locale).find((item) => item.href === `${BASE}/${slug.join("/")}`);
}

export async function generateMetadata({ params }: { params: Params }) {
  return { title: (await itemFor(params))?.label ?? frameMessages[await requestLocale()].placeholder.notFound };
}

export default async function PlaceholderPage({ params }: { params: Params }) {
  const [item, locale] = await Promise.all([itemFor(params), requestLocale()]);
  if (!item) notFound();
  const copy = frameMessages[locale].placeholder;

  return (
    <div className="flex flex-col gap-8">
      <PageHeader title={item.label} description={item.summary} />
      <Card>
        <EmptyState
          icon={item.icon}
          title={copy.title}
          description={copy.description}
          action={
            <Link href={BASE} className={buttonClasses("secondary", "sm")}>
              <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
              {copy.back}
            </Link>
          }
        />
      </Card>
    </div>
  );
}
