import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { FieldSkeleton, PageSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Log streaming" description={120} className="max-w-4xl">
      <Card>
        <CardHeader title="Destination" description="Each event is the same JSON as a webhook's, with secret values already redacted from audit changes." />
        <CardBody className="flex max-w-xl flex-col gap-6">
          <FieldSkeleton label="Protocol" hint={50} />
          <FieldSkeleton hint={20} />
        </CardBody>
      </Card>
    </PageSkeleton>
  );
}
