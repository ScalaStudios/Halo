import { Card } from "@/components/ui/card";
import { FieldSkeleton, PageSkeleton, Skeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Identity providers" description={147} actions={<Skeleton className="h-8 w-44 rounded-md" />}>
      <Card className="p-6">
        <FieldSkeleton
          label="Callback URL"
          hint="Register this exact URL in each provider's app registration: as the redirect URI for Google, Microsoft Entra and OpenID Connect, and as the authorization callback URL for GitHub."
        />
      </Card>
      <TableSkeleton simple columns={6} rows={2} />
    </PageSkeleton>
  );
}
