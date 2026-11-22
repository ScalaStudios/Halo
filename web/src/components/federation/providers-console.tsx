"use client";

import { Ellipsis, LogIn, Network, Pencil, Plus, Power, Trash2, X } from "lucide-react";
import { useRef, useState } from "react";
import { providerStartUrl } from "@/components/auth/providers";
import { useMutation } from "@/components/console/use-mutation";
import { Badge, Tag } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Checkbox, Field, Input, Select, Textarea } from "@/components/ui/input";
import { Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger } from "@/components/ui/menu";
import { PageHeader } from "@/components/ui/page-header";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { cn } from "@/lib/cn";
import type { IdentityProvider, ProfileField, ProviderKind, ProviderMapping } from "@/lib/federation-types";
import { useMessages } from "@/lib/i18n/client";
import { frameMessages } from "@/lib/i18n/console/frame";
import { settingsMessages } from "@/lib/i18n/console/settings";
import type { Group } from "@/lib/types";

const KIND: Record<ProviderKind, { label: string; name: string; scopes: string }> = {
  google: { label: "Google", name: "Google", scopes: "openid email profile" },
  microsoft: { label: "Microsoft Entra ID", name: "Microsoft", scopes: "openid email profile" },
  github: { label: "GitHub", name: "GitHub", scopes: "read:user user:email" },
  oidc: { label: "OpenID Connect", name: "", scopes: "openid email profile" },
  saml: { label: "SAML 2.0", name: "", scopes: "" },
};

const FIELDS: ProfileField[] = ["name", "title", "department", "location"];

type Source = "url" | "xml" | "values";

const SOURCES: Source[] = ["url", "xml", "values"];

const ENTRA = /^https:\/\/login\.microsoftonline\.com\/([^/]+)\/v2\.0$/;
const TEST_NEXT = "/admin/identity-providers";

function words(value: string) {
  return value
    .split(/[\s,]+/)
    .map((word) => word.trim())
    .filter(Boolean);
}

export function ProvidersConsole({ callbackUrl, providers, groups }: { callbackUrl: string; providers: IdentityProvider[]; groups: Group[] }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [editing, setEditing] = useState<IdentityProvider | "new" | null>(null);
  const [deleting, setDeleting] = useState<IdentityProvider | null>(null);
  const enabled = providers.filter((p) => p.enabled).length;
  const { providers: t, common } = useMessages(settingsMessages);
  const title = useMessages(frameMessages).nav.items.identityProviders.label;
  const add = (
    <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      {t.add}
    </Button>
  );

  function toggle(p: IdentityProvider) {
    void run(p.id, () => api(`/identity-providers/${p.id}/${p.enabled ? "disable" : "enable"}`, { method: "POST" }), () =>
      toast({
        title: p.enabled ? t.turnedOff(p.name) : t.turnedOn(p.name),
        description: p.enabled ? t.offHint : t.onHint,
      }),
    );
  }

  function remove() {
    const p = deleting;
    if (!p) return;
    void run("delete", () => api(`/identity-providers/${p.id}`, { method: "DELETE" }), () => {
      setDeleting(null);
      toast({ title: t.deleted(p.name), description: t.deletedHint(p.linkedCount) });
    }, "inline");
  }

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title={title}
        description={t.description(providers.length, enabled)}
        actions={add}
      />
      <Card className="p-6">
        <CopyField
          label={t.callback}
          value={callbackUrl}
          hint={t.callbackHint}
        />
      </Card>
      {providers.length ? (
        <SimpleTable
          caption={title}
          head={[common.provider, common.status, t.emailDomains, t.newPeople, t.linkedAccounts, common.actions]}
          rows={providers.map((p) => [
            <span key="name" className="flex min-w-48 flex-col">
              <span className="font-medium text-fg">{p.name}</span>
              <span className="text-caption text-fg-3">{KIND[p.kind].label}</span>
            </span>,
            <span key="status" className="flex flex-col items-start gap-1">
              <Badge tone={p.enabled ? "success" : "neutral"} dot>
                {p.enabled ? t.on : t.off}
              </Badge>
              {p.enabled && !p.showOnSignIn ? <span className="text-caption whitespace-nowrap text-fg-3">{t.hidden}</span> : null}
              {p.enabled && p.routeByDomain ? <span className="text-caption whitespace-nowrap text-fg-3">{t.routes}</span> : null}
            </span>,
            <span key="domains" className="flex flex-wrap gap-1">
              {p.allowedDomains.length ? p.allowedDomains.map((d) => <Tag key={d}>{d}</Tag>) : <span className="text-fg-3">{t.anyDomain}</span>}
            </span>,
            <span key="jit" className="whitespace-nowrap">
              {p.jit ? t.jitOn : t.jitOff}
            </span>,
            <span key="linked" className="tnum">
              {p.linkedCount}
            </span>,
            <Menu key="actions">
              <MenuTrigger asChild>
                <Button size="icon-sm" variant="quiet" aria-label={t.actionsFor(p.name)} loading={pending === p.id}>
                  <Ellipsis aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
              </MenuTrigger>
              <MenuContent>
                <MenuItem icon={Pencil} onSelect={() => setEditing(p)}>
                  {t.edit}
                </MenuItem>
                <MenuItem icon={LogIn} disabled={!p.enabled} onSelect={() => window.location.assign(providerStartUrl(p.id, undefined, TEST_NEXT))}>
                  {t.test}
                </MenuItem>
                <MenuItem icon={Power} onSelect={() => toggle(p)}>
                  {p.enabled ? t.turnOff : t.turnOn}
                </MenuItem>
                <MenuSeparator />
                <MenuItem icon={Trash2} danger onSelect={() => setDeleting(p)}>
                  {t.delete}
                </MenuItem>
              </MenuContent>
            </Menu>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={Network}
            title={t.emptyTitle}
            description={t.emptyDescription}
            action={add}
          />
        </Card>
      )}
      {providers.length ? (
        <p className="text-caption text-fg-3">{t.testNote}</p>
      ) : null}
      {editing ? (
        <ProviderDialog
          key={editing === "new" ? "new" : editing.id}
          provider={editing === "new" ? null : editing}
          callbackUrl={callbackUrl}
          groups={groups}
          onClose={() => setEditing(null)}
        />
      ) : null}
      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (open) return;
          setDeleting(null);
          clearError();
        }}
        error={errorFor("delete")}
        title={t.deleteTitle(deleting?.name ?? t.deleteFallback)}
        description={t.deleteDescription(deleting?.linkedCount ?? 0, deleting?.name ?? t.itFallback)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              {common.cancel}
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              {t.deleteButton}
            </Button>
          </>
        }
      />
    </div>
  );
}

function ProviderDialog({ provider, callbackUrl, groups, onClose }: { provider: IdentityProvider | null; callbackUrl: string; groups: Group[]; onClose: () => void }) {
  const { pending, run, errorFor, toast } = useMutation();
  const [kind, setKind] = useState<ProviderKind>(provider?.kind ?? "google");
  const [name, setName] = useState(provider?.name ?? KIND.google.name);
  const [issuer, setIssuer] = useState(provider?.kind === "microsoft" ? "" : (provider?.issuer ?? ""));
  const [tenant, setTenant] = useState(provider?.issuer.match(ENTRA)?.[1] ?? "");
  const [clientId, setClientId] = useState(provider?.clientId ?? "");
  const [clientSecret, setClientSecret] = useState("");
  const [scopes, setScopes] = useState(provider?.scopes.join(" ") ?? KIND.google.scopes);
  const [domains, setDomains] = useState(provider?.allowedDomains.join(" ") ?? "");
  const [enabled, setEnabled] = useState(provider?.enabled ?? true);
  const [showOnSignIn, setShowOnSignIn] = useState(provider?.showOnSignIn ?? false);
  const [routeByDomain, setRouteByDomain] = useState(provider?.routeByDomain ?? false);
  const [jit, setJit] = useState(provider?.jit ?? false);
  const [jitGroupIds, setJitGroupIds] = useState(provider?.jitGroupIds ?? []);
  const [profileMap, setProfileMap] = useState<ProviderMapping["profile"]>(provider?.mapping.profile ?? []);
  const [groupClaim, setGroupClaim] = useState(provider?.mapping.groupClaim || "groups");
  const [groupMap, setGroupMap] = useState<ProviderMapping["groups"]>(provider?.mapping.groups ?? []);
  const [source, setSource] = useState<Source>(provider?.metadataXml ? "xml" : "url");
  const [metadataUrl, setMetadataUrl] = useState("");
  const [metadataXml, setMetadataXml] = useState(provider?.metadataXml ?? "");
  const [ssoUrl, setSsoUrl] = useState("");
  const [certificate, setCertificate] = useState("");
  const [emailAttribute, setEmailAttribute] = useState(provider?.emailAttribute ?? "");
  const [errors, setErrors] = useState<
    Partial<
      Record<
        "name" | "issuer" | "tenant" | "clientId" | "clientSecret" | "domains" | "metadata" | "ssoUrl" | "certificate" | "profileMap" | "groupClaim" | "groupMap",
        string
      >
    >
  >({});
  const file = useRef<HTMLInputElement>(null);
  const { providers: t, common } = useMessages(settingsMessages);
  const mapped = kind !== "github";
  const unusedFields = FIELDS.filter((f) => !profileMap.some((m) => m.field === f));
  const assigned = groups.filter((g) => g.kind === "assigned");
  const saml = kind === "saml";
  const secretRequired = !provider?.hasSecret && kind !== "oidc" && !saml;
  const serviceProvider = provider && saml ? `${callbackUrl.replace(/\/api\/v1\/auth\/federated\/callback$/, "")}/saml/sp/${provider.id}` : null;

  function pick(next: ProviderKind) {
    if (name === KIND[kind].name) setName(KIND[next].name);
    if (scopes === KIND[kind].scopes) setScopes(KIND[next].scopes);
    setKind(next);
    setErrors({});
  }

  function save() {
    const next = {
      name: name.trim() ? undefined : t.errors.name,
      tenant: kind === "microsoft" && !tenant.trim() ? t.errors.tenant : undefined,
      issuer:
        kind === "oidc" && !issuer.trim()
          ? t.errors.issuer
          : saml && source === "values" && !issuer.trim()
            ? t.errors.entityId
            : undefined,
      metadata:
        saml && source === "url" && !metadataUrl.trim()
          ? t.errors.metadataUrl
          : saml && source === "xml" && !metadataXml.trim()
            ? t.errors.metadataXml
            : undefined,
      ssoUrl: saml && source === "values" && !ssoUrl.trim() ? t.errors.ssoUrl : undefined,
      certificate: saml && source === "values" && !certificate.trim() ? t.errors.certificate : undefined,
      clientId: saml || clientId.trim() ? undefined : t.errors.clientId,
      clientSecret: secretRequired && !clientSecret.trim() ? t.errors.clientSecret : undefined,
      domains:
        words(domains).length > 0
          ? undefined
          : jit
            ? t.errors.domainsJit
            : routeByDomain
              ? t.errors.domainsRoute
              : undefined,
      profileMap: mapped && profileMap.some((m) => !m.claim.trim()) ? t.errors.profileMap : undefined,
      groupClaim: mapped && groupMap.length && !groupClaim.trim() ? t.errors.groupClaim : undefined,
      groupMap: mapped && groupMap.some((m) => !m.value.trim() || !m.groupId) ? t.errors.groupMap : undefined,
    };
    setErrors(next);
    if (Object.values(next).some(Boolean)) return;
    const body = {
      kind,
      name: name.trim(),
      issuer: kind === "microsoft" ? `https://login.microsoftonline.com/${tenant.trim()}/v2.0` : issuer.trim(),
      clientId: saml ? "" : clientId.trim(),
      clientSecret: saml ? "" : clientSecret.trim(),
      scopes: saml ? [] : words(scopes),
      metadataUrl: saml && source === "url" ? metadataUrl.trim() : "",
      metadataXml: saml && source === "xml" ? metadataXml : "",
      ssoUrl: saml && source === "values" ? ssoUrl.trim() : "",
      certificate: saml && source === "values" ? certificate : "",
      emailAttribute: saml ? emailAttribute.trim() : "",
      enabled,
      showOnSignIn,
      routeByDomain,
      allowedDomains: words(domains),
      jit,
      jitGroupIds: jit ? jitGroupIds : [],
      mapping: mapped
        ? {
            profile: profileMap.map((m) => ({ ...m, claim: m.claim.trim() })),
            groupClaim: groupMap.length ? groupClaim.trim() : "",
            groups: groupMap.map((m) => ({ ...m, value: m.value.trim() })),
          }
        : { profile: [], groupClaim: "", groups: [] },
    };
    void run(
      "save",
      () =>
        provider
          ? api<IdentityProvider>(`/identity-providers/${provider.id}`, { method: "PUT", body })
          : api<IdentityProvider>("/identity-providers", { body }),
      (saved) => {
        toast({
          title: provider ? t.saved(saved.name) : t.added(saved.name),
          description: saved.showOnSignIn
            ? t.onSignIn
            : saved.routeByDomain
              ? t.routed(saved.allowedDomains.join(", "))
              : saved.kind === "saml" && !provider
                ? t.samlNext
                : t.testNext,
        });
        onClose();
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      error={errorFor("save")}
      title={provider ? t.editTitle(provider.name) : t.add}
      description={saml ? t.samlDescription : t.oauthDescription}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {common.cancel}
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {provider ? t.save : t.addButton}
          </Button>
        </>
      }
    >
      <form
        className="flex flex-col gap-6"
        onSubmit={(event) => {
          event.preventDefault();
          save();
        }}
      >
        {provider ? null : (
          <fieldset className="flex flex-col gap-3">
            <legend className="mb-3 text-label text-fg">{common.provider}</legend>
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              {(Object.keys(KIND) as ProviderKind[]).map((option) => (
                <label
                  key={option}
                  className={cn(
                    "flex cursor-pointer flex-col gap-1 rounded-lg border p-4 transition-colors duration-fast ease-brand has-[:focus-visible]:ring-focus",
                    option === kind ? "border-ember bg-hover" : "border-border-strong hover:bg-hover",
                  )}
                >
                  <input type="radio" name="kind" value={option} checked={option === kind} onChange={() => pick(option)} className="sr-only" />
                  <span className="text-body-sm font-semibold text-fg">{KIND[option].label}</span>
                  <span className="text-caption text-fg-3">{t.kinds[option]}</span>
                </label>
              ))}
            </div>
          </fieldset>
        )}
        <Field label={common.name} hint={t.nameHint} error={errors.name}>
          {(props) => <Input {...props} autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder="Acme SSO" />}
        </Field>
        {kind === "microsoft" ? (
          <Field label={t.tenant} hint={t.tenantHint} error={errors.tenant}>
            {(props) => (
              <Input {...props} mono autoComplete="off" spellCheck={false} value={tenant} onChange={(event) => setTenant(event.target.value)} placeholder="72f988bf-86f1-41af-91ab-2d7cd011db47" />
            )}
          </Field>
        ) : null}
        {kind === "github" ? (
          <Field label={t.githubUrl} hint={t.githubHint}>
            {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={issuer} onChange={(event) => setIssuer(event.target.value)} placeholder="https://github.com" />}
          </Field>
        ) : null}
        {kind === "oidc" ? (
          <Field label={t.issuer} hint={t.issuerHint} error={errors.issuer}>
            {(props) => (
              <Input {...props} mono autoComplete="off" spellCheck={false} value={issuer} onChange={(event) => setIssuer(event.target.value)} placeholder="https://auth.example.com" />
            )}
          </Field>
        ) : null}
        {saml ? (
          <>
            {serviceProvider ? (
              <>
                <CopyField label={t.spMetadata} value={`${serviceProvider}/metadata`} hint={t.spMetadataHint} />
                <CopyField label={t.acs} value={`${serviceProvider}/acs`} hint={t.acsHint} />
              </>
            ) : null}
            <Field label={t.idpSettings}>
              {(props) => (
                <Select {...props} value={source} onChange={(event) => setSource(event.target.value as Source)}>
                  {SOURCES.map((key) => (
                    <option key={key} value={key}>
                      {t.sources[key]}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            {source === "url" ? (
              <Field label={t.metadataUrl} hint={t.metadataUrlHint} error={errors.metadata}>
                {(props) => (
                  <Input
                    {...props}
                    mono
                    autoComplete="off"
                    spellCheck={false}
                    value={metadataUrl}
                    onChange={(event) => setMetadataUrl(event.target.value)}
                    placeholder="https://idp.example.com/saml/metadata"
                  />
                )}
              </Field>
            ) : null}
            {source === "xml" ? (
              <Field label={t.metadataXml} hint={t.metadataXmlHint} error={errors.metadata}>
                {(props) => (
                  <div className="flex flex-col gap-3">
                    <Textarea
                      {...props}
                      mono
                      rows={6}
                      spellCheck={false}
                      value={metadataXml}
                      onChange={(event) => setMetadataXml(event.target.value)}
                      placeholder={'<EntityDescriptor entityID="https://idp.example.com" …>'}
                    />
                    <input
                      ref={file}
                      type="file"
                      accept=".xml,application/xml,text/xml,application/samlmetadata+xml"
                      hidden
                      onChange={(event) => {
                        const chosen = event.target.files?.[0];
                        event.target.value = "";
                        if (chosen) void chosen.text().then(setMetadataXml);
                      }}
                    />
                    <Button size="sm" variant="secondary" className="self-start" onClick={() => file.current?.click()}>
                      {t.chooseFile}
                    </Button>
                  </div>
                )}
              </Field>
            ) : null}
            {source === "values" ? (
              <>
                <Field label={t.entityId} hint={t.entityIdHint} error={errors.issuer}>
                  {(props) => (
                    <Input {...props} mono autoComplete="off" spellCheck={false} value={issuer} onChange={(event) => setIssuer(event.target.value)} placeholder="https://idp.example.com" />
                  )}
                </Field>
                <Field label={t.ssoUrl} hint={t.ssoUrlHint} error={errors.ssoUrl}>
                  {(props) => (
                    <Input
                      {...props}
                      mono
                      autoComplete="off"
                      spellCheck={false}
                      value={ssoUrl}
                      onChange={(event) => setSsoUrl(event.target.value)}
                      placeholder="https://idp.example.com/saml/sso"
                    />
                  )}
                </Field>
                <Field label={t.certificate} hint={t.certificateHint} error={errors.certificate}>
                  {(props) => (
                    <Textarea
                      {...props}
                      mono
                      rows={4}
                      spellCheck={false}
                      value={certificate}
                      onChange={(event) => setCertificate(event.target.value)}
                      placeholder="-----BEGIN CERTIFICATE-----"
                    />
                  )}
                </Field>
              </>
            ) : null}
            <Field label={t.emailAttribute} hint={t.emailAttributeHint}>
              {(props) => (
                <Input {...props} mono autoComplete="off" spellCheck={false} value={emailAttribute} onChange={(event) => setEmailAttribute(event.target.value)} placeholder="NameID" />
              )}
            </Field>
          </>
        ) : (
          <>
            <Field label={t.clientId} error={errors.clientId}>
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={clientId} onChange={(event) => setClientId(event.target.value)} />}
            </Field>
            <Field
              label={t.clientSecret}
              hint={provider?.hasSecret ? common.keepSecret : kind === "oidc" ? t.pkce : common.storedEncrypted}
              error={errors.clientSecret}
            >
              {(props) => (
                <Input {...props} mono type="password" autoComplete="new-password" spellCheck={false} value={clientSecret} onChange={(event) => setClientSecret(event.target.value)} />
              )}
            </Field>
            <Field label={t.scopes} hint={t.scopesHint}>
              {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={scopes} onChange={(event) => setScopes(event.target.value)} />}
            </Field>
          </>
        )}
        <Field
          label={common.allowedDomains}
          hint={t.domainsHint}
          error={errors.domains}
        >
          {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={domains} onChange={(event) => setDomains(event.target.value)} placeholder="example.com" />}
        </Field>
        <fieldset className="flex flex-col gap-4">
          <legend className="mb-3 text-label text-fg">{t.signIn}</legend>
          <Toggle checked={enabled} onChange={setEnabled} label={t.enabled} description={t.enabledHint} />
          <Toggle
            checked={showOnSignIn}
            onChange={setShowOnSignIn}
            label={t.show}
            description={t.showHint}
          />
          <Toggle
            checked={routeByDomain}
            onChange={setRouteByDomain}
            label={t.route}
            description={t.routeHint}
          />
          <Toggle
            checked={jit}
            onChange={setJit}
            label={t.jit}
            description={t.jitHint}
          />
        </fieldset>
        {jit ? (
          <fieldset className="flex animate-enter flex-col gap-3">
            <legend className="mb-3 text-label text-fg">{t.addTo}</legend>
            {assigned.length ? (
              assigned.map((g) => (
                <label key={g.id} className="flex cursor-pointer items-center gap-3 text-body-sm text-fg">
                  <Checkbox
                    checked={jitGroupIds.includes(g.id)}
                    onChange={(event) => setJitGroupIds(event.target.checked ? [...jitGroupIds, g.id] : jitGroupIds.filter((id) => id !== g.id))}
                  />
                  {g.name}
                </label>
              ))
            ) : (
              <p className="text-body-sm text-fg-3">{common.noAssignedGroups}</p>
            )}
          </fieldset>
        ) : null}
        {mapped ? (
          <>
            <fieldset className="flex flex-col gap-3">
              <legend className="mb-3 text-label text-fg">{t.profile}</legend>
              {profileMap.map((m, index) => (
                <div key={index} className="flex items-center gap-2">
                  <Input
                    mono
                    className="min-w-0"
                    aria-label={t.claimFor(t.fields[m.field])}
                    autoComplete="off"
                    spellCheck={false}
                    value={m.claim}
                    onChange={(event) => setProfileMap((current) => current.map((c, i) => (i === index ? { ...c, claim: event.target.value } : c)))}
                    placeholder="department"
                  />
                  <Select
                    aria-label={t.profileField(index + 1)}
                    value={m.field}
                    onChange={(event) =>
                      setProfileMap((current) => current.map((c, i) => (i === index ? { ...c, field: event.target.value as ProfileField } : c)))
                    }
                  >
                    {FIELDS.map((f) => (
                      <option key={f} value={f} disabled={f !== m.field && !unusedFields.includes(f)}>
                        {t.fields[f]}
                      </option>
                    ))}
                  </Select>
                  <Button
                    size="icon"
                    variant="quiet"
                    aria-label={t.stopMapping(t.fields[m.field])}
                    onClick={() => setProfileMap((current) => current.filter((_, i) => i !== index))}
                  >
                    <X aria-hidden="true" size={16} strokeWidth={1.75} />
                  </Button>
                </div>
              ))}
              <Button
                size="sm"
                variant="secondary"
                className="self-start"
                disabled={!unusedFields.length}
                onClick={() => setProfileMap((current) => [...current, { claim: "", field: unusedFields[0] }])}
              >
                <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
                {t.mapField}
              </Button>
              {errors.profileMap ? (
                <p className="text-caption text-danger">{errors.profileMap}</p>
              ) : (
                <p className="text-caption text-fg-3">{t.profileNote}</p>
              )}
            </fieldset>
            <fieldset className="flex flex-col gap-3">
              <legend className="mb-3 text-label text-fg">{t.groups}</legend>
              {assigned.length ? (
                <>
                  {groupMap.length ? (
                    <Field label={t.groupsClaim} error={errors.groupClaim}>
                      {(props) => (
                        <Input
                          {...props}
                          mono
                          autoComplete="off"
                          spellCheck={false}
                          value={groupClaim}
                          onChange={(event) => setGroupClaim(event.target.value)}
                          placeholder="groups"
                        />
                      )}
                    </Field>
                  ) : null}
                  {groupMap.map((m, index) => (
                    <div key={index} className="flex items-center gap-2">
                      <Input
                        mono
                        className="min-w-0"
                        aria-label={t.providerValue(index + 1)}
                        autoComplete="off"
                        spellCheck={false}
                        value={m.value}
                        onChange={(event) => setGroupMap((current) => current.map((c, i) => (i === index ? { ...c, value: event.target.value } : c)))}
                        placeholder={t.valuePlaceholder}
                      />
                      <Select
                        aria-label={t.haloGroup(index + 1)}
                        value={m.groupId}
                        onChange={(event) => setGroupMap((current) => current.map((c, i) => (i === index ? { ...c, groupId: event.target.value } : c)))}
                      >
                        <option value="" disabled>
                          {common.chooseGroup}
                        </option>
                        {assigned.map((g) => (
                          <option key={g.id} value={g.id}>
                            {g.name}
                          </option>
                        ))}
                      </Select>
                      <Button
                        size="icon"
                        variant="quiet"
                        aria-label={t.removeRow(index + 1)}
                        onClick={() => setGroupMap((current) => current.filter((_, i) => i !== index))}
                      >
                        <X aria-hidden="true" size={16} strokeWidth={1.75} />
                      </Button>
                    </div>
                  ))}
                  <Button
                    size="sm"
                    variant="secondary"
                    className="self-start"
                    disabled={groupMap.length >= 100}
                    onClick={() => setGroupMap((current) => [...current, { value: "", groupId: "" }])}
                  >
                    <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
                    {t.mapGroup}
                  </Button>
                  {errors.groupMap ? (
                    <p className="text-caption text-danger">{errors.groupMap}</p>
                  ) : (
                    <p className="text-caption text-fg-3">{t.groupsNote}</p>
                  )}
                </>
              ) : (
                <p className="text-body-sm text-fg-3">{t.noAssignedMap}</p>
              )}
            </fieldset>
          </>
        ) : null}
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

function Toggle({ checked, onChange, label, description }: { checked: boolean; onChange: (checked: boolean) => void; label: string; description: string }) {
  return (
    <label className="flex cursor-pointer items-start gap-3">
      <Checkbox className="mt-0.5" checked={checked} onChange={(event) => onChange(event.target.checked)} />
      <span className="flex flex-col">
        <span className="text-body-sm text-fg">{label}</span>
        <span className="text-caption text-fg-3">{description}</span>
      </span>
    </label>
  );
}
