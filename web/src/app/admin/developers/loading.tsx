import { FileCode2 } from "lucide-react";
import { buttonClasses } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { FieldSkeleton, PageSkeleton, SkeletonText } from "@/components/ui/skeleton";

const endpoints = ["Issuer", "Discovery document", "Authorization endpoint", "Token endpoint", "Userinfo endpoint", "JWKS", "SAML metadata", "SCIM base URL"];

export default function Loading() {
  return (
    <PageSkeleton
      title="SDKs and reference"
      description="Halo speaks standard OpenID Connect, SAML 2.0 and SCIM 2.0, so any certified client library works. These are the addresses to give it."
      className="max-w-5xl"
    >
      <Card>
        <CardHeader title="Protocol endpoints" description="Most OpenID Connect libraries need only the issuer; they read the rest from the discovery document." />
        <CardBody className="grid grid-cols-1 gap-6 md:grid-cols-2">
          {endpoints.map((label) => (
            <FieldSkeleton key={label} label={label} />
          ))}
        </CardBody>
      </Card>
      <Card>
        <CardHeader
          title="Management API"
          description="Everything in this console is available over JSON at the API base URL."
          actions={
            <a href="/api/v1/openapi.yaml" className={buttonClasses("secondary", "sm")}>
              <FileCode2 aria-hidden="true" size={16} strokeWidth={1.75} />
              OpenAPI document
            </a>
          }
        />
        <CardBody className="flex flex-col gap-6">
          <FieldSkeleton label="API base URL" />
          <p className="max-w-2xl text-body-sm">
            <SkeletonText chars={330} />
          </p>
        </CardBody>
      </Card>
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
        <Card className="min-w-0">
          <CardHeader title="List people with curl" description="Reads the directory with a key stored in HALO_API_KEY." />
          <CardBody>
            <div className="h-28 rounded-md border border-border bg-sunken" />
          </CardBody>
        </Card>
        <Card className="min-w-0">
          <CardHeader title="List people from Go" description="The same request with the standard library." />
          <CardBody>
            <div className="h-96 rounded-md border border-border bg-sunken" />
          </CardBody>
        </Card>
      </div>
    </PageSkeleton>
  );
}
