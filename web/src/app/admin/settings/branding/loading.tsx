import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { FieldSkeleton, PageSkeleton, Skeleton } from "@/components/ui/skeleton";

export default function Loading() {
  return (
    <PageSkeleton
      title="Branding"
      description="Your logo, colours and a short message on the pages people see when they sign in, set up a passkey or manage their account."
      className="max-w-4xl"
    >
      <Card>
        <CardHeader title="Logo" description="Shown with your organization name above the sign-in and setup forms. Halo's own mark stays in the footer." />
        <CardBody className="flex flex-col gap-4">
          <div className="h-36 rounded-md border border-border bg-sunken" />
          <Skeleton className="h-10 w-36 rounded-md" />
        </CardBody>
      </Card>
      <Card>
        <CardHeader title="Sign-in message" description="A short note under the logo on the sign-in, magic link and setup pages, such as where to get help." />
        <CardBody>
          <div className="flex max-w-xl flex-col gap-6">
            <FieldSkeleton label="Message" hint={60} tall />
            <Skeleton className="h-10 w-36 rounded-md" />
          </div>
        </CardBody>
      </Card>
      <Card>
        <CardHeader
          title="Colours"
          description="Used on the sign-in, setup and account pages. Hover, pressed and secondary shades are mixed from them. Leave a colour empty to keep Halo's."
        />
        <CardBody>
          <div className="h-64 rounded-md border border-border bg-sunken" />
        </CardBody>
      </Card>
    </PageSkeleton>
  );
}
