"use client";

import Link from "next/link";
import { Check, FileKey, KeyRound, ServerCog, type LucideIcon } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Button, buttonClasses } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { Field, Input, Select, Textarea } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { cn } from "@/lib/cn";
import { NAME_ID_FORMATS, type NameIdFormat, type SamlApplication } from "@/lib/saml-types";
import { endpoints, setupGuide } from "@/lib/setup-guides";
import { SamlConnectionFields } from "./saml";
import { SecretWarning, SetupGuideCard, TYPE_ICON } from "./shared";

type Protocol = "oidc" | "saml" | "oauth";
type Source = "values" | "metadata";
type Platform = "web" | "spa" | "native";
type Template = { value: string; label: string; uri: string; entityId?: string; guide: string; platform?: Platform; hint?: string; more?: string[]; scopes?: string[] };
type Option<T extends string> = { value: T; label: string; description: string; icon?: LucideIcon };

const PROTOCOLS: Option<Protocol>[] = [
  { value: "oidc", label: "OpenID Connect", description: "Sign-in for web, single-page and command-line apps. Use this unless the app only supports SAML.", icon: KeyRound },
  { value: "saml", label: "SAML 2.0", description: "Sign-in for applications that only support SAML, like AWS IAM Identity Center and Slack.", icon: FileKey },
  { value: "oauth", label: "OAuth 2.0 service", description: "Machine-to-machine access with client credentials. Nobody signs in.", icon: ServerCog },
];

const PLATFORMS: Option<Platform>[] = [
  { value: "web", label: "Web application", description: "Runs on a server and keeps a client secret, like Grafana or Nextcloud.", icon: TYPE_ICON.web },
  { value: "spa", label: "Single-page app", description: "Runs entirely in the browser. Signs in with PKCE and has no secret.", icon: TYPE_ICON.spa },
  { value: "native", label: "Native or CLI", description: "Desktop, mobile and command-line tools like kubectl. Redirects to localhost with PKCE.", icon: TYPE_ICON.native },
];

const TEMPLATES: Template[] = [
  { value: "grafana", label: "Grafana", uri: "https://grafana.example.com/login/generic_oauth", guide: "grafana" },
  { value: "forgejo", label: "Forgejo", uri: "https://git.example.com/user/oauth2/halo/callback", guide: "forgejo" },
  { value: "nextcloud", label: "Nextcloud", uri: "https://cloud.example.com/apps/user_oidc/code", guide: "generic-oidc" },
  { value: "proxmox", label: "Proxmox VE", uri: "https://pve.example.com:8006", guide: "generic-oidc" },
  { value: "outline", label: "Outline", uri: "https://wiki.example.com/auth/oidc.callback", guide: "generic-oidc" },
  { value: "headscale", label: "Headscale", uri: "https://hs.example.com/oidc/callback", guide: "generic-oidc" },
  {
    value: "kubernetes",
    label: "Kubernetes",
    hint: "kubectl oidc-login",
    uri: "http://localhost:8000",
    more: ["http://localhost:18000"],
    scopes: ["openid", "profile", "email", "groups", "offline_access"],
    guide: "kubernetes",
    platform: "native",
  },
  { value: "custom", label: "Custom", uri: "", guide: "generic-oidc" },
];

const SAML_TEMPLATES: Template[] = [
  {
    value: "aws",
    label: "AWS IAM Identity Center",
    entityId: "https://eu-west-1.signin.aws.amazon.com/platform/saml/d-0000000000",
    uri: "https://eu-west-1.signin.aws.amazon.com/platform/saml/acs/00000000-0000-0000-0000-000000000000",
    guide: "aws-iam-identity-center",
  },
  { value: "slack", label: "Slack", entityId: "https://slack.com", uri: "https://example.slack.com/sso/saml", guide: "slack" },
  { value: "zammad", label: "Zammad", entityId: "https://support.example.com/auth/saml/metadata", uri: "https://support.example.com/auth/saml/callback", guide: "generic-saml" },
  { value: "custom", label: "Custom", entityId: "", uri: "", guide: "generic-saml" },
];

const SOURCES: Option<Source>[] = [
  { value: "values", label: "Enter values", description: "Entity ID and ACS URL" },
  { value: "metadata", label: "Paste metadata", description: "The service provider's metadata XML" },
];

const PLACEHOLDER: Record<Platform, string> = {
  web: "https://app.example.com/oidc/callback",
  spa: "https://app.example.com/callback",
  native: "http://localhost:8000",
};

export function uriError(value: string, empty = "Enter where Halo sends people after they sign in, like https://app.example.com/callback."): string | undefined {
  const raw = value.trim();
  if (!raw) return empty;
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return "This is not a full URL. Include https:// and the host, like https://app.example.com/callback.";
  }
  const local = ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname);
  if (url.protocol !== "https:" && !(url.protocol === "http:" && local)) {
    return "Use https:// so sign-in responses are never sent in plain text. Plain http:// is only allowed for localhost.";
  }
  if (url.hash) return `Remove “${url.hash}” from the end. Halo cannot redirect to a URI with a fragment.`;
  return undefined;
}

export function NewApplicationFlow({ issuer }: { issuer: string }) {
  const [step, setStep] = useState(0);
  const [protocol, setProtocol] = useState<Protocol>("oidc");
  const [platform, setPlatform] = useState<Platform>("web");
  const [template, setTemplate] = useState("custom");
  const [name, setName] = useState("");
  const [uri, setUri] = useState("");
  const [entityId, setEntityId] = useState("");
  const [source, setSource] = useState<Source>("values");
  const [metadata, setMetadata] = useState("");
  const [format, setFormat] = useState<NameIdFormat>("email");
  const [errors, setErrors] = useState<{ name?: string; uri?: string; entityId?: string; metadata?: string }>({});
  const [created, setCreated] = useState<{ application: SamlApplication; clientSecret: string | null } | null>(null);
  const [guideOpen, setGuideOpen] = useState(false);
  const { pending, run } = useMutation();

  const steps = protocol === "oidc" ? ["Protocol", "Platform", "Details"] : protocol === "saml" ? ["Protocol", "Service provider", "Details"] : ["Protocol", "Details"];
  const current = steps[step];
  const needsUri = protocol !== "oauth";
  const urls = endpoints(issuer);
  const picked = TEMPLATES.find((t) => t.value === template);
  const platformTemplates = TEMPLATES.filter((t) => t.value === "custom" || (t.platform ?? "web") === platform);

  function pickTemplate(value: string, list = TEMPLATES) {
    const picked = list.find((t) => t.value === value);
    setTemplate(value);
    setName(value === "custom" ? "" : (picked?.label ?? ""));
    setUri(picked?.uri ?? "");
    setEntityId(picked?.entityId ?? "");
    setErrors({});
  }

  function pickProtocol(value: Protocol) {
    setProtocol(value);
    if (template !== "custom") pickTemplate("custom");
  }

  function pickPlatform(value: Platform) {
    setPlatform(value);
    if (template !== "custom") pickTemplate("custom");
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    if (current !== "Details") {
      setStep(step + 1);
      return;
    }
    if (protocol === "saml") {
      submitSaml();
      return;
    }
    const next = {
      name: name.trim() ? undefined : "Enter a name. People see it on the Halo sign-in screen.",
      uri: needsUri ? uriError(uri) : undefined,
    };
    setErrors(next);
    if (next.name || next.uri) return;
    void run(
      "create",
      () =>
        api<{ application: SamlApplication; clientSecret: string | null }>("/applications", {
          body: {
            name: name.trim(),
            protocol,
            type: protocol === "oauth" ? "service" : platform,
            redirectUris: needsUri ? [uri.trim(), ...(picked?.more ?? [])] : [],
            scopes: picked?.scopes,
            setupGuide: picked?.guide ?? "generic-oidc",
          },
        }),
      setCreated,
    );
  }

  function submitSaml() {
    const values = source === "values";
    const next = {
      name: name.trim() ? undefined : "Enter a name. People see it on the Halo sign-in screen.",
      entityId: values && !entityId.trim() ? "Enter the entity ID from the application's SAML settings, like https://slack.com." : undefined,
      uri: values ? uriError(uri, "Enter the ACS URL, where Halo posts the SAML response, like https://app.example.com/saml/acs.") : undefined,
      metadata: !values && !metadata.trim() ? "Paste the service provider's metadata XML. Applications usually offer it as a download or at a metadata URL." : undefined,
    };
    setErrors(next);
    if (next.name || next.entityId || next.uri || next.metadata) return;
    void run(
      "create",
      () =>
        api<{ application: SamlApplication; clientSecret: string | null }>("/applications", {
          body: {
            name: name.trim(),
            protocol,
            setupGuide: SAML_TEMPLATES.find((t) => t.value === template)?.guide ?? "generic-saml",
            saml: values ? { entityId: entityId.trim(), acsUrls: [uri.trim()], nameIdFormat: format } : { metadataXml: metadata, nameIdFormat: format },
          },
        }),
      setCreated,
    );
  }

  if (created) {
    const { application, clientSecret } = created;
    const appName = application.name;
    const guide = setupGuide(application, issuer);
    return (
      <div className="flex animate-enter flex-col gap-6">
        <Card>
          <CardHeader
            title="Application created"
            description={
              application.saml
                ? `Give ${appName} these values so it trusts Halo, or point it at the metadata URL.`
                : clientSecret
                  ? `Add these values to ${appName} to finish connecting it to Halo.`
                  : `Add these values to ${appName}. It is a public client, so it has no secret and must sign in with PKCE.`
            }
          />
          <CardBody className="flex flex-col gap-4">
            {application.saml ? (
              <>
                <SamlConnectionFields idp={application.saml.idp} />
                <CopyField label="SP entity ID" value={application.clientId} />
              </>
            ) : (
              <>
                <CopyField label="Issuer" value={urls.issuer} />
                <CopyField label="Client ID" value={application.clientId} />
                {clientSecret ? <CopyField label="Client secret" value={clientSecret} secret /> : null}
                <CopyField label={protocol === "oauth" ? "Token endpoint" : "Discovery endpoint"} value={protocol === "oauth" ? urls.token : urls.discovery} />
              </>
            )}
            {application.redirectUris.map((redirect) => (
              <CopyField key={redirect} label={application.saml ? "ACS URL" : "Redirect URI"} value={redirect} />
            ))}
            {clientSecret ? (
              <SecretWarning>This is the only time Halo shows the client secret. Store it in your secret manager now. If you lose it, rotate it from the Credentials tab.</SecretWarning>
            ) : null}
          </CardBody>
        </Card>
        <div className="flex flex-wrap justify-end gap-2">
          <Button variant="secondary" aria-expanded={guideOpen} onClick={() => setGuideOpen(!guideOpen)}>
            {guideOpen ? "Hide setup guide" : "View setup guide"}
          </Button>
          <Link href={`/admin/applications/${application.id}`} className={buttonClasses("primary")}>
            Open application
          </Link>
        </div>
        {guideOpen ? <SetupGuideCard guide={guide} className="animate-enter" /> : null}
      </div>
    );
  }

  return (
    <Card>
      <ol aria-label="Steps" className="flex gap-6 overflow-x-auto border-b border-border px-6">
        {steps.map((label, index) => (
          <li
            key={label}
            aria-current={index === step ? "step" : undefined}
            className={cn(
              "relative flex h-12 shrink-0 items-center gap-2 text-body-sm font-medium",
              index === step ? "text-fg" : index < step ? "text-fg-2" : "text-fg-3",
            )}
          >
            {index < step ? <Check aria-hidden="true" size={16} strokeWidth={1.75} className="text-fg-3" /> : <span className="tnum">{index + 1}</span>}
            {label}
            {index === step ? <span aria-hidden="true" className="absolute inset-x-0 bottom-0 h-0.5 rounded-full bg-ember" /> : null}
          </li>
        ))}
      </ol>
      <form onSubmit={submit} noValidate>
        <div key={current} className="flex animate-enter flex-col gap-8 p-6">
          {current === "Protocol" ? (
            <Choices legend="Choose a protocol" name="protocol" options={PROTOCOLS} value={protocol} onChange={pickProtocol} />
          ) : null}
          {current === "Platform" ? (
            <>
              <Choices legend="Where does the application run?" name="platform" options={PLATFORMS} value={platform} onChange={pickPlatform} />
              {platformTemplates.length > 1 ? (
                <Choices
                  legend="Start from a template"
                  hint="Fills in the name and redirect URI. You can change both on the next step."
                  name="template"
                  compact
                  options={platformTemplates.map((t) => ({ value: t.value, label: t.label, description: t.hint ?? (t.uri ? new URL(t.uri).host : "Your own redirect URI") }))}
                  value={template}
                  onChange={pickTemplate}
                />
              ) : null}
            </>
          ) : null}
          {current === "Service provider" ? (
            <>
              <Choices
                legend="Start from a template"
                hint="Fills in the name, entity ID and ACS URL. You can change them on the next step."
                name="template"
                compact
                options={SAML_TEMPLATES.map((t) => ({ value: t.value, label: t.label, description: t.uri ? new URL(t.uri).host : "Your own service provider" }))}
                value={template}
                onChange={(value) => pickTemplate(value, SAML_TEMPLATES)}
              />
              <Choices legend="How do you want to add it?" name="source" compact options={SOURCES} value={source} onChange={setSource} />
            </>
          ) : null}
          {current === "Details" && protocol === "saml" ? (
            <div className="flex flex-col gap-6">
              <h2 className="text-h4 text-fg">Application details</h2>
              <Field label="Name" error={errors.name}>
                {(props) => <Input {...props} autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="Team wiki" />}
              </Field>
              {source === "values" ? (
                <>
                  <Field label="Entity ID" error={errors.entityId} hint="The service provider's issuer, from its SAML settings or metadata.">
                    {(props) => <Input {...props} mono value={entityId} onChange={(e) => setEntityId(e.target.value)} placeholder="https://app.example.com/saml/metadata" />}
                  </Field>
                  <Field label="ACS URL" error={errors.uri} hint="Where Halo posts the SAML response. Must use https, except for localhost.">
                    {(props) => <Input {...props} mono value={uri} onChange={(e) => setUri(e.target.value)} placeholder="https://app.example.com/saml/acs" />}
                  </Field>
                </>
              ) : (
                <Field label="Metadata XML" error={errors.metadata} hint="Halo reads the entity ID and every HTTP-POST ACS URL from it.">
                  {(props) => (
                    <Textarea
                      {...props}
                      mono
                      rows={8}
                      value={metadata}
                      onChange={(e) => setMetadata(e.target.value)}
                      placeholder={'<EntityDescriptor entityID="https://app.example.com/saml/metadata" …>'}
                    />
                  )}
                </Field>
              )}
              <Field label="NameID format" hint={`Halo sends ${NAME_ID_FORMATS[format].value} as the NameID. Most applications expect the email address.`}>
                {(props) => (
                  <Select {...props} value={format} onChange={(e) => setFormat(e.target.value as NameIdFormat)}>
                    {(Object.keys(NAME_ID_FORMATS) as NameIdFormat[]).map((key) => (
                      <option key={key} value={key}>
                        {NAME_ID_FORMATS[key].label}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
            </div>
          ) : null}
          {current === "Details" && protocol !== "saml" ? (
            <div className="flex flex-col gap-6">
              <h2 className="text-h4 text-fg">Application details</h2>
              <Field label="Name" error={errors.name}>
                {(props) => <Input {...props} autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="Team wiki" />}
              </Field>
              {needsUri ? (
                <Field
                  label="Redirect URI"
                  error={errors.uri}
                  hint={
                    picked?.more
                      ? `Halo also registers ${picked.more.join(", ")} for when the first port is busy. Edit redirect URIs later on the Authentication tab.`
                      : "Where Halo sends people after they sign in. Must use https, except for localhost."
                  }
                >
                  {(props) => <Input {...props} mono value={uri} onChange={(e) => setUri(e.target.value)} placeholder={PLACEHOLDER[platform]} />}
                </Field>
              ) : null}
            </div>
          ) : null}
        </div>
        <div className="flex items-center justify-between gap-4 border-t border-border px-6 py-4">
          {step === 0 ? (
            <Link href="/admin/applications" className={buttonClasses("quiet")}>
              Cancel
            </Link>
          ) : (
            <Button variant="secondary" onClick={() => setStep(step - 1)}>
              Back
            </Button>
          )}
          <Button type="submit" variant="primary" loading={pending === "create"}>
            {current === "Details" ? "Create application" : "Continue"}
          </Button>
        </div>
      </form>
    </Card>
  );
}

function Choices<T extends string>({
  legend,
  hint,
  name,
  options,
  value,
  onChange,
  compact = false,
}: {
  legend: string;
  hint?: string;
  name: string;
  options: Option<T>[];
  value: T;
  onChange: (value: T) => void;
  compact?: boolean;
}) {
  return (
    <fieldset>
      <legend className="font-display text-h4 text-fg">{legend}</legend>
      {hint ? <p className="mt-1 text-body-sm text-fg-3">{hint}</p> : null}
      <div className={cn("mt-4 grid gap-3", compact && "grid-cols-2 sm:grid-cols-3 lg:grid-cols-4")}>
        {options.map((option) => {
          const Icon = option.icon;
          const selected = option.value === value;
          return (
            <label
              key={option.value}
              className={cn(
                "flex cursor-pointer items-start gap-4 rounded-lg border transition-colors duration-fast ease-brand has-[:focus-visible]:ring-focus",
                compact ? "p-4" : "p-6",
                selected ? "border-ember bg-hover" : "border-border-strong hover:bg-hover",
              )}
            >
              <input type="radio" name={name} value={option.value} checked={selected} onChange={() => onChange(option.value)} className="sr-only" />
              {Icon ? <Icon aria-hidden="true" size={24} strokeWidth={1.75} className="shrink-0 text-fg-3" /> : null}
              <span className="flex min-w-0 flex-col gap-1">
                <span className="text-body-sm font-semibold text-fg">{option.label}</span>
                <span className={cn("text-fg-3", compact ? "truncate text-caption" : "text-body-sm")}>{option.description}</span>
              </span>
            </label>
          );
        })}
      </div>
    </fieldset>
  );
}
