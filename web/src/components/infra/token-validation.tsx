import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { infrastructureMessages } from "@/lib/i18n/console/infrastructure";
import { requestLocale } from "@/lib/i18n/server";
import { endpoints } from "@/lib/setup-guides";

export async function TokenValidationCard({ issuer, identifier, algorithm }: { issuer: string; identifier: string; algorithm: string }) {
  const t = infrastructureMessages[await requestLocale()].tokens;
  const code = (text: string) => <code key={text} className="font-mono text-code-sm text-fg">{text}</code>;
  const checks = [t.signature(algorithm), t.iss(code, issuer), t.aud(code, identifier), t.exp(code), t.scope(code), t.sub(code)];
  return (
    <Card>
      <CardHeader title={t.title} description={t.hint} />
      <CardBody className="flex flex-col gap-6">
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <CopyField label={t.issuer} value={issuer} />
          <CopyField label={t.jwks} value={endpoints(issuer).jwks} />
        </div>
        <ol className="flex list-decimal flex-col gap-2 pl-6 text-body-sm text-fg-2 marker:text-fg-3">
          {checks.map((check, i) => (
            <li key={i}>{check}</li>
          ))}
        </ol>
      </CardBody>
    </Card>
  );
}
