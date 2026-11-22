"use client";

import Link from "next/link";
import { Bookmark, BookUser, Check, FileKey, KeyRound, Network, ServerCog, ShieldCheck, SquareTerminal, type LucideIcon } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Button, buttonClasses } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { Field, Input, Select, Textarea } from "@/components/ui/input";
import { remoteBody, remoteDraft, remoteErrors, RemoteFields, type RemoteErrors } from "@/components/remote/settings";
import { api } from "@/lib/api/client";
import { cn } from "@/lib/cn";
import { useLocale } from "@/lib/i18n/client";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import type { Locale } from "@/lib/i18n/locale";
import type { Outpost } from "@/lib/infra-types";
import { NAME_ID_FORMATS, type NameIdFormat, type SamlApplication } from "@/lib/saml-types";
import { endpoints, setupGuide } from "@/lib/setup-guides";
import { SamlConnectionFields } from "./saml";
import { LdapConnectionFields, RadiusServerFields, SecretWarning, SetupGuideCard, TYPE_ICON } from "./shared";

type Protocol = "oidc" | "saml" | "oauth" | "forward-auth" | "radius" | "ldap" | "remote" | "bookmark";
type Source = "values" | "metadata";
type Platform = "web" | "spa" | "native";
type Template = { value: string; label: string; uri: string; entityId?: string; guide: string; platform?: Platform; hint?: string; more?: string[]; scopes?: string[] };
type Option<T extends string> = { value: T; label: string; description: string; icon?: LucideIcon };
type Step = "protocol" | "platform" | "provider" | "details";

const PROTOCOL_ICON: Record<Protocol, LucideIcon> = {
  oidc: KeyRound,
  saml: FileKey,
  oauth: ServerCog,
  "forward-auth": ShieldCheck,
  radius: Network,
  ldap: BookUser,
  remote: SquareTerminal,
  bookmark: Bookmark,
};

const TEMPLATES: Template[] = [
  { value: "grafana", label: "Grafana", uri: "https://grafana.example.com/login/generic_oauth", guide: "grafana" },
  { value: "forgejo", label: "Forgejo", uri: "https://git.example.com/user/oauth2/halo/callback", guide: "forgejo" },
  { value: "aether", label: "Aether", uri: "https://mail.example.com/api/v1/auth/oidc/halo/callback", scopes: ["openid", "profile", "email", "groups"], guide: "aether" },
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

const PLACEHOLDER: Record<Platform, string> = {
  web: "https://app.example.com/oidc/callback",
  spa: "https://app.example.com/callback",
  native: "http://localhost:8000",
};

export function uriError(value: string, locale: Locale, empty = applicationsMessages[locale].setup.validation.uriEmpty): string | undefined {
  const v = applicationsMessages[locale].setup.validation;
  const raw = value.trim();
  if (!raw) return empty;
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return v.notUrl;
  }
  const local = ["localhost", "127.0.0.1", "[::1]"].includes(url.hostname);
  if (url.protocol !== "https:" && !(url.protocol === "http:" && local)) {
    return v.https;
  }
  if (url.hash) return v.fragment(url.hash);
  return undefined;
}

export function originError(value: string, locale: Locale): string | undefined {
  const v = applicationsMessages[locale].setup.validation;
  const error = uriError(value, locale, v.originEmpty);
  if (error) return error;
  const url = new URL(value.trim());
  return url.pathname !== "/" || url.search ? v.originPath : undefined;
}

export function clientAddressError(value: string, locale: Locale): string | undefined {
  const v = applicationsMessages[locale].setup.validation;
  const raw = value.trim();
  if (!raw) return v.clientAddressEmpty;
  return /^[0-9a-f.:]+(\/\d{1,3})?$/i.test(raw) ? undefined : v.clientAddress;
}

export function NewApplicationFlow({ issuer, featuresOff, outposts }: { issuer: string; featuresOff: string[]; outposts: Outpost[] }) {
  const [step, setStep] = useState(0);
  const [protocol, setProtocol] = useState<Protocol>("oidc");
  const [platform, setPlatform] = useState<Platform>("web");
  const [template, setTemplate] = useState("custom");
  const [name, setName] = useState("");
  const [uri, setUri] = useState("");
  const [address, setAddress] = useState("");
  const [slug, setSlug] = useState("halo");
  const [entityId, setEntityId] = useState("");
  const [source, setSource] = useState<Source>("values");
  const [metadata, setMetadata] = useState("");
  const [format, setFormat] = useState<NameIdFormat>("email");
  const [errors, setErrors] = useState<{ name?: string; uri?: string; address?: string; slug?: string; entityId?: string; metadata?: string }>({});
  const [created, setCreated] = useState<{ application: SamlApplication; clientSecret: string | null } | null>(null);
  const [guideOpen, setGuideOpen] = useState(false);
  const [remote, setRemote] = useState(() => remoteDraft(outposts));
  const [remoteProblems, setRemoteProblems] = useState<RemoteErrors>({});
  const { pending, run, errorFor, router, toast } = useMutation();
  const locale = useLocale();
  const t = applicationsMessages[locale];
  const s = t.setup;
  const v = s.validation;

  const steps: Step[] = protocol === "oidc" ? ["protocol", "platform", "details"] : protocol === "saml" ? ["protocol", "provider", "details"] : ["protocol", "details"];
  const current = steps[step];
  const needsUri = protocol !== "oauth";
  const urls = endpoints(issuer);
  const picked = TEMPLATES.find((item) => item.value === template);
  const platformTemplates = TEMPLATES.filter((item) => item.value === "custom" || (item.platform ?? "web") === platform);
  const aether = template === "aether";
  const base = address.trim().replace(/\/+$/, "");

  function pickTemplate(value: string, list = TEMPLATES) {
    const picked = list.find((item) => item.value === value);
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
    if (current !== "details") {
      setStep(step + 1);
      return;
    }
    if (protocol === "saml") {
      submitSaml();
      return;
    }
    if (protocol === "bookmark") {
      submitBookmark();
      return;
    }
    if (protocol === "forward-auth") {
      submitForwardAuth();
      return;
    }
    if (protocol === "radius") {
      submitRadius();
      return;
    }
    if (protocol === "ldap") {
      submitLdap();
      return;
    }
    if (protocol === "remote") {
      submitRemote();
      return;
    }
    const next = {
      name: name.trim() ? undefined : v.nameSignIn,
      uri: needsUri && !aether ? uriError(uri, locale) : undefined,
      address: aether ? uriError(address, locale, v.aetherAddress) : undefined,
      slug: aether && !/^[a-z0-9][a-z0-9-]{0,62}$/.test(slug.trim()) ? v.slug : undefined,
    };
    setErrors(next);
    if (next.name || next.uri || next.address || next.slug) return;
    void run(
      "create",
      () =>
        api<{ application: SamlApplication; clientSecret: string | null }>("/applications", {
          body: {
            name: name.trim(),
            protocol,
            type: protocol === "oauth" ? "service" : platform,
            redirectUris: aether ? [`${base}/api/v1/auth/oidc/${slug.trim()}/callback`] : needsUri ? [uri.trim(), ...(picked?.more ?? [])] : [],
            postLogoutUris: aether ? [`${base}/login`] : undefined,
            scopes: picked?.scopes,
            setupGuide: picked?.guide ?? "generic-oidc",
          },
        }),
      setCreated,
      "inline",
    );
  }

  function submitBookmark() {
    const url = address.trim();
    const next = {
      name: name.trim() ? undefined : v.namePortal,
      address: !url
        ? v.bookmarkEmpty
        : /^https?:\/\/[^/\s]+/.test(url)
          ? undefined
          : v.bookmarkInvalid,
    };
    setErrors(next);
    if (next.name || next.address) return;
    void run(
      "create",
      () => api<{ application: SamlApplication }>("/applications", { body: { name: name.trim(), protocol, homepage: url } }),
      ({ application }) => {
        toast({ title: t.common.createdToast(application.name), description: s.bookmarkCreated });
        router.push(`/admin/applications/${application.id}?tab=users`);
      },
      "inline",
    );
  }

  function submitForwardAuth() {
    const next = { name: name.trim() ? undefined : v.nameSignIn, address: originError(address, locale) };
    setErrors(next);
    if (next.name || next.address) return;
    void run(
      "create",
      () => api<{ application: SamlApplication }>("/applications", { body: { name: name.trim(), protocol, redirectUris: [address.trim()] } }),
      ({ application }) => {
        toast({ title: t.common.createdToast(application.name), description: s.forwardCreated });
        router.push(`/admin/applications/${application.id}`);
      },
      "inline",
    );
  }

  function submitRadius() {
    const next = { name: name.trim() ? undefined : v.nameLog, address: clientAddressError(address, locale) };
    setErrors(next);
    if (next.name || next.address) return;
    void run(
      "create",
      () => api<{ application: SamlApplication; clientSecret: string | null }>("/applications", { body: { name: name.trim(), protocol, redirectUris: [address.trim()] } }),
      setCreated,
      "inline",
    );
  }

  function submitLdap() {
    const next = { name: name.trim() ? undefined : v.nameLdap };
    setErrors(next);
    if (next.name) return;
    void run(
      "create",
      () => api<{ application: SamlApplication; clientSecret: string | null }>("/applications", { body: { name: name.trim(), protocol } }),
      setCreated,
      "inline",
    );
  }

  function submitRemote() {
    const next = { name: name.trim() ? undefined : v.namePortal };
    const problems = remoteErrors(remote, false);
    setErrors(next);
    setRemoteProblems(problems);
    if (next.name || Object.values(problems).some(Boolean)) return;
    void run(
      "create",
      () => api<{ application: SamlApplication }>("/applications", { body: { name: name.trim(), protocol, remote: remoteBody(remote) } }),
      ({ application }) => {
        toast({ title: t.common.createdToast(application.name), description: s.remoteCreated });
        router.push(`/admin/applications/${application.id}?tab=users`);
      },
      "inline",
    );
  }

  function submitSaml() {
    const values = source === "values";
    const next = {
      name: name.trim() ? undefined : v.nameSignIn,
      entityId: values && !entityId.trim() ? v.entityId : undefined,
      uri: values ? uriError(uri, locale, v.acsEmpty) : undefined,
      metadata: !values && !metadata.trim() ? v.metadata : undefined,
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
            setupGuide: SAML_TEMPLATES.find((item) => item.value === template)?.guide ?? "generic-saml",
            saml: values ? { entityId: entityId.trim(), acsUrls: [uri.trim()], nameIdFormat: format } : { metadataXml: metadata, nameIdFormat: format },
          },
        }),
      setCreated,
      "inline",
    );
  }

  if (created) {
    const { application, clientSecret } = created;
    const appName = application.name;
    const radius = application.protocol === "radius";
    const guide = setupGuide(application, issuer);
    return (
      <div className="flex animate-enter flex-col gap-6">
        <Card>
          <CardHeader
            title={s.created}
            description={
              application.ldap
                ? s.createdLdap(appName)
                : application.saml
                  ? s.createdSaml(appName)
                  : radius
                    ? s.createdRadius(appName)
                    : clientSecret
                      ? s.createdSecret(appName)
                      : s.createdPublic(appName)
            }
          />
          <CardBody className="flex flex-col gap-4">
            {application.ldap ? (
              <>
                <LdapConnectionFields ldap={application.ldap} locale={locale} />
                {clientSecret ? <CopyField label={t.fields.bindPassword} value={clientSecret} secret /> : null}
              </>
            ) : application.saml ? (
              <>
                <SamlConnectionFields idp={application.saml.idp} />
                <CopyField label={t.fields.spEntityId} value={application.clientId} />
              </>
            ) : radius ? (
              <>
                <RadiusServerFields issuer={issuer} port={application.radiusPort} locale={locale} />
                {clientSecret ? <CopyField label={t.fields.sharedSecret} value={clientSecret} secret /> : null}
              </>
            ) : (
              <>
                <CopyField label={t.fields.issuer} value={urls.issuer} />
                <CopyField label={t.fields.clientId} value={application.clientId} />
                {clientSecret ? <CopyField label={t.fields.clientSecret} value={clientSecret} secret /> : null}
                <CopyField label={protocol === "oauth" ? t.fields.tokenEndpoint : t.fields.discovery} value={protocol === "oauth" ? urls.token : urls.discovery} />
              </>
            )}
            {application.redirectUris.map((redirect) => (
              <CopyField key={redirect} label={application.saml ? t.fields.acsUrl : radius ? t.fields.clientAddress : t.fields.redirectUri} value={redirect} />
            ))}
            {clientSecret && application.ldap ? (
              <SecretWarning>{s.bindOnce}</SecretWarning>
            ) : clientSecret ? (
              <SecretWarning>{radius ? s.sharedOnce : s.clientOnce}</SecretWarning>
            ) : null}
          </CardBody>
        </Card>
        <div className="flex flex-wrap justify-end gap-2">
          {radius || application.ldap ? null : (
            <Button variant="secondary" aria-expanded={guideOpen} onClick={() => setGuideOpen(!guideOpen)}>
              {guideOpen ? s.hideGuide : s.viewGuide}
            </Button>
          )}
          <Link href={`/admin/applications/${application.id}`} className={buttonClasses("primary")}>
            {s.open}
          </Link>
        </div>
        {guideOpen ? <SetupGuideCard guide={guide} className="animate-enter" /> : null}
      </div>
    );
  }

  return (
    <Card>
      <ol aria-label={s.stepsLabel} className="flex gap-6 overflow-x-auto border-b border-border px-6">
        {steps.map((key, index) => (
          <li
            key={key}
            aria-current={index === step ? "step" : undefined}
            className={cn(
              "relative flex h-12 shrink-0 items-center gap-2 text-body-sm font-medium",
              index === step ? "text-fg" : index < step ? "text-fg-2" : "text-fg-3",
            )}
          >
            {index < step ? <Check aria-hidden="true" size={16} strokeWidth={1.75} className="text-fg-3" /> : <span className="tnum">{index + 1}</span>}
            {s.steps[key]}
            {index === step ? <span aria-hidden="true" className="absolute inset-x-0 bottom-0 h-0.5 rounded-full bg-ember" /> : null}
          </li>
        ))}
      </ol>
      <form onSubmit={submit} noValidate>
        <div key={current} className="flex animate-enter flex-col gap-8 p-6">
          {current === "protocol" ? (
            <Choices
              legend={s.chooseProtocol}
              name="protocol"
              options={(Object.keys(PROTOCOL_ICON) as Protocol[])
                .filter((value) => !featuresOff.includes(value === "remote" ? "remote-access" : value))
                .map((value) => ({ value, ...s.protocols[value], icon: PROTOCOL_ICON[value] }))}
              value={protocol}
              onChange={pickProtocol}
            />
          ) : null}
          {current === "platform" ? (
            <>
              <Choices
                legend={s.whereRuns}
                name="platform"
                options={(["web", "spa", "native"] as const).map((value) => ({ value, ...s.platforms[value], icon: TYPE_ICON[value] }))}
                value={platform}
                onChange={pickPlatform}
              />
              {platformTemplates.length > 1 ? (
                <Choices
                  legend={s.template}
                  hint={s.templateHint}
                  name="template"
                  compact
                  options={platformTemplates.map((item) => ({ value: item.value, label: item.value === "custom" ? s.custom : item.label, description: item.hint ?? (item.uri ? new URL(item.uri).host : s.ownRedirect) }))}
                  value={template}
                  onChange={pickTemplate}
                />
              ) : null}
            </>
          ) : null}
          {current === "provider" ? (
            <>
              <Choices
                legend={s.template}
                hint={s.samlTemplateHint}
                name="template"
                compact
                options={SAML_TEMPLATES.map((item) => ({ value: item.value, label: item.value === "custom" ? s.custom : item.label, description: item.uri ? new URL(item.uri).host : s.ownProvider }))}
                value={template}
                onChange={(value) => pickTemplate(value, SAML_TEMPLATES)}
              />
              <Choices legend={s.howAdd} name="source" compact options={(["values", "metadata"] as const).map((value) => ({ value, ...s.sources[value] }))} value={source} onChange={setSource} />
            </>
          ) : null}
          {current === "details" && protocol === "saml" ? (
            <div className="flex flex-col gap-6">
              <h2 className="text-h4 text-fg">{s.applicationDetails}</h2>
              <Field label={t.common.name} error={errors.name}>
                {(props) => <Input {...props} autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder={s.teamWiki} />}
              </Field>
              {source === "values" ? (
                <>
                  <Field label={s.entityId} error={errors.entityId} hint={s.entityIdHint}>
                    {(props) => <Input {...props} mono value={entityId} onChange={(e) => setEntityId(e.target.value)} placeholder="https://app.example.com/saml/metadata" />}
                  </Field>
                  <Field label={t.fields.acsUrl} error={errors.uri} hint={s.acsHint}>
                    {(props) => <Input {...props} mono value={uri} onChange={(e) => setUri(e.target.value)} placeholder="https://app.example.com/saml/acs" />}
                  </Field>
                </>
              ) : (
                <Field label={s.metadataXml} error={errors.metadata} hint={s.metadataHint}>
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
              <Field label={s.nameIdFormat} hint={s.nameIdHint(NAME_ID_FORMATS[format].value)}>
                {(props) => (
                  <Select {...props} value={format} onChange={(e) => setFormat(e.target.value as NameIdFormat)}>
                    {(Object.keys(NAME_ID_FORMATS) as NameIdFormat[]).map((key) => (
                      <option key={key} value={key}>
                        {s.nameIdFormats[key]}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
            </div>
          ) : null}
          {current === "details" && protocol === "forward-auth" ? (
            <div className="flex flex-col gap-6">
              <h2 className="text-h4 text-fg">{s.applicationDetails}</h2>
              <Field label={t.common.name} error={errors.name}>
                {(props) => <Input {...props} autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder={s.fileServer} />}
              </Field>
              <Field label={t.fields.protectedAddress} error={errors.address} hint={s.protectedHint}>
                {(props) => <Input {...props} type="url" mono value={address} onChange={(e) => setAddress(e.target.value)} placeholder="https://files.example.com" />}
              </Field>
            </div>
          ) : null}
          {current === "details" && protocol === "radius" ? (
            <div className="flex flex-col gap-6">
              <h2 className="text-h4 text-fg">{s.radiusDetails}</h2>
              <Field label={t.common.name} error={errors.name}>
                {(props) => <Input {...props} autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder={s.officeVpn} />}
              </Field>
              <Field label={t.fields.clientAddress} error={errors.address} hint={s.clientAddressHint}>
                {(props) => <Input {...props} mono value={address} onChange={(e) => setAddress(e.target.value)} placeholder="10.0.0.5" />}
              </Field>
            </div>
          ) : null}
          {current === "details" && protocol === "ldap" ? (
            <div className="flex flex-col gap-6">
              <h2 className="text-h4 text-fg">{s.applicationDetails}</h2>
              <Field label={t.common.name} error={errors.name} hint={s.ldapNameHint}>
                {(props) => <Input {...props} autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder="Proxmox VE" />}
              </Field>
            </div>
          ) : null}
          {current === "details" && protocol === "remote" ? (
            <div className="flex flex-col gap-6">
              <h2 className="text-h4 text-fg">{s.remoteDetails}</h2>
              <Field label={t.common.name} error={errors.name} hint={s.remoteNameHint}>
                {(props) => <Input {...props} autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder={s.databaseHost} />}
              </Field>
              <RemoteFields draft={remote} onChange={setRemote} errors={remoteProblems} outposts={outposts} />
            </div>
          ) : null}
          {current === "details" && protocol === "bookmark" ? (
            <div className="flex flex-col gap-6">
              <h2 className="text-h4 text-fg">{s.bookmarkDetails}</h2>
              <Field label={t.common.name} error={errors.name}>
                {(props) => <Input {...props} autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder={s.statusPage} />}
              </Field>
              <Field label={t.common.url} error={errors.address} hint={s.bookmarkUrlHint}>
                {(props) => <Input {...props} type="url" mono value={address} onChange={(e) => setAddress(e.target.value)} placeholder="https://status.example.com" />}
              </Field>
            </div>
          ) : null}
          {current === "details" && (protocol === "oidc" || protocol === "oauth") ? (
            <div className="flex flex-col gap-6">
              <h2 className="text-h4 text-fg">{s.applicationDetails}</h2>
              <Field label={t.common.name} error={errors.name}>
                {(props) => <Input {...props} autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder={s.teamWiki} />}
              </Field>
              {aether ? (
                <>
                  <Field label={s.aetherAddress} error={errors.address} hint={s.aetherAddressHint}>
                    {(props) => <Input {...props} type="url" mono value={address} onChange={(e) => setAddress(e.target.value)} placeholder="https://mail.example.com" />}
                  </Field>
                  <Field label={s.slug} error={errors.slug} hint={s.slugHint}>
                    {(props) => <Input {...props} mono value={slug} onChange={(e) => setSlug(e.target.value)} placeholder="halo" />}
                  </Field>
                </>
              ) : needsUri ? (
                <Field label={t.fields.redirectUri} error={errors.uri} hint={picked?.more ? s.moreRedirects(picked.more.join(", ")) : s.redirectHint}>
                  {(props) => <Input {...props} mono value={uri} onChange={(e) => setUri(e.target.value)} placeholder={PLACEHOLDER[platform]} />}
                </Field>
              ) : null}
            </div>
          ) : null}
        </div>
        <ActionError message={errorFor("create")} className="px-6 pb-4" />
        <div className="flex items-center justify-between gap-4 border-t border-border px-6 py-4">
          {step === 0 ? (
            <Link href="/admin/applications" className={buttonClasses("quiet")}>
              {t.common.cancel}
            </Link>
          ) : (
            <Button variant="secondary" onClick={() => setStep(step - 1)}>
              {t.common.back}
            </Button>
          )}
          <Button type="submit" variant="primary" loading={pending === "create"}>
            {current === "details" ? (protocol === "bookmark" ? s.createBookmark : s.createApplication) : t.common.continue}
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
