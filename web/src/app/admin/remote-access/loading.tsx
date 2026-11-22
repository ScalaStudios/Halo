import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { SectionTitle } from "@/components/ui/section-title";
import { PageSkeleton, Skeleton, TableSkeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton title="Remote access" description={150}>
      <div className="flex flex-col gap-6">
        <SectionTitle
          title="Outposts"
          description="Agents inside your networks that connect out to Halo and reach hosts for it. Each remote access application names the outpost that reaches its host."
          action={<Skeleton className="h-8 w-36 rounded-md" />}
        />
        <TableSkeleton simple columns={5} rows={2} />
      </div>
      <Card>
        <CardHeader
          title="Run an outpost"
          description="Run halo-outpost on a machine that reaches your hosts, with the token from Register outpost. It keeps one HTTPS connection open to Halo and opens a TCP connection to a host only when a session to it starts. Then add a remote access application for each host."
        />
        <CardBody>
          <div className="h-40 rounded-md border border-border bg-sunken" />
        </CardBody>
      </Card>
      <div className="flex flex-col gap-6">
        <SectionTitle
          title="Sessions"
          description="The 200 most recent sessions, newest first. Halo keeps them, with their recordings, for as long as the sign-in log. Ending a session closes it at once."
        />
        <TableSkeleton simple columns={6} rows={4} />
      </div>
    </PageSkeleton>
  );
}
