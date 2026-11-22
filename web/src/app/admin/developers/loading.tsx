import { FileCode2 } from "lucide-react";
import { buttonClasses } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { FieldSkeleton, PageSkeleton, SkeletonText } from "@/components/ui/skeleton";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";

export default async function Loading() {
  const locale = await requestLocale();
  const t = applicationsMessages[locale].developers;
  const endpoints = Object.values(t.endpoints);
  return (
    <PageSkeleton
      title={frameMessages[locale].nav.items.developers.label}
      description={t.description}
      className="max-w-5xl"
    >
      <Card>
        <CardHeader title={t.protocol} description={t.protocolHint} />
        <CardBody className="grid grid-cols-1 gap-6 md:grid-cols-2">
          {endpoints.map((label) => (
            <FieldSkeleton key={label} label={label} />
          ))}
        </CardBody>
      </Card>
      <Card>
        <CardHeader
          title={t.api}
          description={t.apiHint}
          actions={
            <a href="/api/v1/openapi.yaml" className={buttonClasses("secondary", "sm")}>
              <FileCode2 aria-hidden="true" size={16} strokeWidth={1.75} />
              {t.openapi}
            </a>
          }
        />
        <CardBody className="flex flex-col gap-6">
          <FieldSkeleton label={t.baseUrl} />
          <p className="max-w-2xl text-body-sm">
            <SkeletonText chars={330} />
          </p>
        </CardBody>
      </Card>
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
        <Card className="min-w-0">
          <CardHeader title={t.curl} description={t.curlHint} />
          <CardBody>
            <div className="h-28 rounded-md border border-border bg-sunken" />
          </CardBody>
        </Card>
        <Card className="min-w-0">
          <CardHeader title={t.go} description={t.goHint} />
          <CardBody>
            <div className="h-96 rounded-md border border-border bg-sunken" />
          </CardBody>
        </Card>
      </div>
    </PageSkeleton>
  );
}
