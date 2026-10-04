import Link from "next/link";
import { FileCode2 } from "lucide-react";
import { SetupGuideCard } from "@/components/applications/shared";
import { buttonClasses } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { endpoints } from "@/lib/setup-guides";
import type { OrganizationProfile } from "@/lib/settings-types";

export const metadata = { title: "SDKs and reference" };

export default async function DevelopersPage() {
  const { issuer } = await apiGet<OrganizationProfile>("/organization");
  const urls = endpoints(issuer);
  const api = `${issuer}/api/v1`;
  const protocol = [
    { label: "Issuer", value: urls.issuer },
    { label: "Discovery document", value: urls.discovery },
    { label: "Authorization endpoint", value: urls.authorize },
    { label: "Token endpoint", value: urls.token },
    { label: "Userinfo endpoint", value: urls.userinfo },
    { label: "JWKS", value: urls.jwks },
    { label: "SAML metadata", value: urls.samlMetadata },
    { label: "SCIM base URL", value: `${issuer}/scim/v2` },
  ];

  return (
    <div className="flex max-w-5xl animate-page flex-col gap-8">
      <PageHeader
        title="SDKs and reference"
        description="Halo speaks standard OpenID Connect, SAML 2.0 and SCIM 2.0, so any certified client library works. These are the addresses to give it."
      />
      <Card>
        <CardHeader title="Protocol endpoints" description="Most OpenID Connect libraries need only the issuer; they read the rest from the discovery document." />
        <CardBody className="grid grid-cols-1 gap-6 md:grid-cols-2">
          {protocol.map((item) => (
            <CopyField key={item.label} label={item.label} value={item.value} />
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
          <CopyField label="API base URL" value={api} />
          <p className="max-w-2xl text-body-sm text-fg-2">
            Authenticate with an API key in the Authorization header: <code className="font-mono text-code-sm text-fg">Authorization: Bearer hlk_…</code>. Create keys for a{" "}
            <Link href="/admin/service-accounts" className="text-link underline underline-offset-3 hover:text-ember">
              service account
            </Link>{" "}
            with the api scope. A key can do exactly what the service account&apos;s roles allow, and every change it makes appears in the audit log under that account&apos;s name.
          </p>
        </CardBody>
      </Card>
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
        <SetupGuideCard
          className="min-w-0"
          guide={{
            title: "List people with curl",
            description: "Reads the directory with a key stored in HALO_API_KEY.",
            file: "shell",
            code: `curl -sS ${api}/users \\\n  -H "Authorization: Bearer $HALO_API_KEY"`,
          }}
        />
        <SetupGuideCard
          className="min-w-0"
          guide={{
            title: "List people from Go",
            description: "The same request with the standard library.",
            file: "main.go",
            code: [
              `req, err := http.NewRequest("GET", "${api}/users", nil)`,
              "if err != nil {",
              "\treturn err",
              "}",
              `req.Header.Set("Authorization", "Bearer "+os.Getenv("HALO_API_KEY"))`,
              "res, err := http.DefaultClient.Do(req)",
              "if err != nil {",
              "\treturn err",
              "}",
              "defer res.Body.Close()",
              "var people []struct {",
              '\tID    string `json:"id"`',
              '\tEmail string `json:"email"`',
              '\tName  string `json:"name"`',
              "}",
              "return json.NewDecoder(res.Body).Decode(&people)",
            ].join("\n"),
          }}
        />
      </div>
    </div>
  );
}
