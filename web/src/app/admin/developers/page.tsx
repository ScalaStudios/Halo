import Link from "next/link";
import { FileCode2 } from "lucide-react";
import { SetupGuideCard } from "@/components/applications/shared";
import { buttonClasses } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { PageHeader } from "@/components/ui/page-header";
import { apiGet } from "@/lib/api/server";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import { frameMessages } from "@/lib/i18n/console/frame";
import { requestLocale } from "@/lib/i18n/server";
import { endpoints } from "@/lib/setup-guides";
import type { OrganizationProfile } from "@/lib/settings-types";

export async function generateMetadata() {
  return { title: frameMessages[await requestLocale()].nav.items.developers.label };
}

export default async function DevelopersPage() {
  const [{ issuer }, locale] = await Promise.all([apiGet<OrganizationProfile>("/organization"), requestLocale()]);
  const t = applicationsMessages[locale].developers;
  const urls = endpoints(issuer);
  const api = `${issuer}/api/v1`;
  const protocol = [
    { label: t.endpoints.issuer, value: urls.issuer },
    { label: t.endpoints.discovery, value: urls.discovery },
    { label: t.endpoints.authorize, value: urls.authorize },
    { label: t.endpoints.token, value: urls.token },
    { label: t.endpoints.userinfo, value: urls.userinfo },
    { label: t.endpoints.jwks, value: urls.jwks },
    { label: t.endpoints.samlMetadata, value: urls.samlMetadata },
    { label: t.endpoints.scim, value: `${issuer}/scim/v2` },
  ];

  return (
    <div className="flex max-w-5xl flex-col gap-8">
      <PageHeader
        title={frameMessages[locale].nav.items.developers.label}
        description={t.description}
      />
      <Card>
        <CardHeader title={t.protocol} description={t.protocolHint} />
        <CardBody className="grid grid-cols-1 gap-6 md:grid-cols-2">
          {protocol.map((item) => (
            <CopyField key={item.label} label={item.label} value={item.value} />
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
          <CopyField label={t.baseUrl} value={api} />
          <p className="max-w-2xl text-body-sm text-fg-2">
            {t.authenticate} <code className="font-mono text-code-sm text-fg">Authorization: Bearer hlk_…</code>
            {t.createKeys}{" "}
            <Link href="/admin/service-accounts" className="text-link underline underline-offset-3 hover:text-ember">
              {t.serviceAccount}
            </Link>{" "}
            {t.keyScope}
          </p>
        </CardBody>
      </Card>
      <div className="grid grid-cols-1 gap-6 xl:grid-cols-2">
        <SetupGuideCard
          className="min-w-0"
          guide={{
            title: t.curl,
            description: t.curlHint,
            file: "shell",
            code: `curl -sS ${api}/users \\\n  -H "Authorization: Bearer $HALO_API_KEY"`,
          }}
        />
        <SetupGuideCard
          className="min-w-0"
          guide={{
            title: t.go,
            description: t.goHint,
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
