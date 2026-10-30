import Link from "next/link";
import { Bot } from "lucide-react";
import { buttonClasses } from "@/components/ui/button";
import { PageSkeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton
      title="API keys"
      description={142}
      actions={
        <Link href="/admin/service-accounts" className={buttonClasses("secondary", "sm")}>
          <Bot aria-hidden="true" size={16} strokeWidth={1.75} />
          Service accounts
        </Link>
      }
    >
      <TableSkeleton columns={8} views={2} facets={2} rows={3} />
    </PageSkeleton>
  );
}
