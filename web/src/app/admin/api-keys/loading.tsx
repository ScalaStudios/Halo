import Link from "next/link";
import { Bot } from "lucide-react";
import { buttonClasses } from "@/components/ui/button";
import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const nav = frameMessages[await requestLocale()].nav.items;
  return (
    <PageSkeleton
      title={nav.apiKeys.label}
      description={142}
      actions={
        <Link href="/admin/service-accounts" className={buttonClasses("secondary", "sm")}>
          <Bot aria-hidden="true" size={16} strokeWidth={1.75} />
          {nav.serviceAccounts.label}
        </Link>
      }
    >
      <TableSkeleton columns={9} views={2} facets={2} rows={3} />
    </PageSkeleton>
  );
}
