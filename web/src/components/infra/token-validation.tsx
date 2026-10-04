import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { endpoints } from "@/lib/setup-guides";

export function TokenValidationCard({ issuer, identifier }: { issuer: string; identifier: string }) {
  const checks = [
    <>Verify the RS256 signature with the key from the JWKS whose kid matches the token header. Cache the key set and fetch it again when an unknown kid appears.</>,
    <>
      Check that <code className="font-mono text-code-sm text-fg">iss</code> equals <code className="font-mono text-code-sm text-fg">{issuer}</code>.
    </>,
    <>
      Check that <code className="font-mono text-code-sm text-fg">aud</code> contains <code className="font-mono text-code-sm text-fg">{identifier}</code>.
    </>,
    <>
      Check that <code className="font-mono text-code-sm text-fg">exp</code> is in the future, allowing a minute of clock skew.
    </>,
    <>
      Check that <code className="font-mono text-code-sm text-fg">scope</code> contains the scope the endpoint needs. ID tokens never carry a scope claim, so this check also refuses an ID
      token sent in place of an access token.
    </>,
    <>
      Use <code className="font-mono text-code-sm text-fg">sub</code> as the caller: a user ID, or the client ID for service applications.{" "}
      <code className="font-mono text-code-sm text-fg">client_id</code> names the application that requested the token.
    </>,
  ];
  return (
    <Card>
      <CardHeader title="Validate tokens in the API" description="Access tokens for this API are JWTs signed by Halo. The API validates them itself, without calling Halo on each request." />
      <CardBody className="flex flex-col gap-6">
        <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <CopyField label="Issuer" value={issuer} />
          <CopyField label="JWKS URL" value={endpoints(issuer).jwks} />
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
