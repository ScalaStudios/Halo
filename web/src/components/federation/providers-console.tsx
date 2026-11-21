"use client";

import { Ellipsis, LogIn, Network, Pencil, Plus, Power, Trash2, X } from "lucide-react";
import { useState } from "react";
import { providerStartUrl } from "@/components/auth/providers";
import { useMutation } from "@/components/console/use-mutation";
import { Badge, Tag } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Checkbox, Field, Input, Select } from "@/components/ui/input";
import { Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger } from "@/components/ui/menu";
import { PageHeader } from "@/components/ui/page-header";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { cn } from "@/lib/cn";
import type { IdentityProvider, ProfileField, ProviderKind, ProviderMapping } from "@/lib/federation-types";
import { pluralize } from "@/lib/format";
import type { Group } from "@/lib/types";

const KIND: Record<ProviderKind, { label: string; name: string; scopes: string; description: string }> = {
  google: { label: "Google", name: "Google", scopes: "openid email profile", description: "Google Workspace and Google accounts." },
  microsoft: { label: "Microsoft Entra ID", name: "Microsoft", scopes: "openid email profile", description: "Work accounts from one Entra tenant." },
  github: { label: "GitHub", name: "GitHub", scopes: "read:user user:email", description: "GitHub.com or GitHub Enterprise Server." },
  oidc: { label: "OpenID Connect", name: "", scopes: "openid email profile", description: "Okta, Keycloak, Authentik or any OIDC provider." },
};

const FIELD: Record<ProfileField, string> = { name: "Name", title: "Job title", department: "Department", location: "Location" };

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
  const add = (
    <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
      <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
      Add identity provider
    </Button>
  );

  function toggle(p: IdentityProvider) {
    void run(p.id, () => api(`/identity-providers/${p.id}/${p.enabled ? "disable" : "enable"}`, { method: "POST" }), () =>
      toast({
        title: `${p.name} turned ${p.enabled ? "off" : "on"}`,
        description: p.enabled ? "Its sign-in button is gone and its linked accounts can't sign in." : "People with a linked account can sign in with it again.",
      }),
    );
  }

  function remove() {
    const p = deleting;
    if (!p) return;
    void run("delete", () => api(`/identity-providers/${p.id}`, { method: "DELETE" }), () => {
      setDeleting(null);
      toast({ title: `${p.name} deleted`, description: `${pluralize(p.linkedCount, "linked account")} unlinked. Recorded in the audit log.` });
    }, "inline");
  }

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        title="Identity providers"
        description={`${pluralize(providers.length, "provider")}, ${enabled} turned on. People sign in with their account at these providers; Halo still applies your access policies and application assignments.`}
        actions={add}
      />
      <Card className="p-6">
        <CopyField
          label="Callback URL"
          value={callbackUrl}
          hint="Register this exact URL in each provider's app registration: as the redirect URI for Google, Microsoft Entra and OpenID Connect, and as the authorization callback URL for GitHub."
        />
      </Card>
      {providers.length ? (
        <SimpleTable
          caption="Identity providers"
          head={["Provider", "Status", "Email domains", "New people", "Linked accounts", "Actions"]}
          rows={providers.map((p) => [
            <span key="name" className="flex min-w-48 flex-col">
              <span className="font-medium text-fg">{p.name}</span>
              <span className="text-caption text-fg-3">{KIND[p.kind].label}</span>
            </span>,
            <span key="status" className="flex flex-col items-start gap-1">
              <Badge tone={p.enabled ? "success" : "neutral"} dot>
                {p.enabled ? "On" : "Off"}
              </Badge>
              {p.enabled && !p.showOnSignIn ? <span className="text-caption whitespace-nowrap text-fg-3">Hidden from sign-in page</span> : null}
              {p.enabled && p.routeByDomain ? <span className="text-caption whitespace-nowrap text-fg-3">Routes by email domain</span> : null}
            </span>,
            <span key="domains" className="flex flex-wrap gap-1">
              {p.allowedDomains.length ? p.allowedDomains.map((d) => <Tag key={d}>{d}</Tag>) : <span className="text-fg-3">Any domain</span>}
            </span>,
            <span key="jit" className="whitespace-nowrap">
              {p.jit ? "Created on first sign-in" : "Existing accounts only"}
            </span>,
            <span key="linked" className="tnum">
              {p.linkedCount}
            </span>,
            <Menu key="actions">
              <MenuTrigger asChild>
                <Button size="icon-sm" variant="quiet" aria-label={`Actions for ${p.name}`} loading={pending === p.id}>
                  <Ellipsis aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
              </MenuTrigger>
              <MenuContent>
                <MenuItem icon={Pencil} onSelect={() => setEditing(p)}>
                  Edit
                </MenuItem>
                <MenuItem icon={LogIn} disabled={!p.enabled} onSelect={() => window.location.assign(providerStartUrl(p.id, undefined, TEST_NEXT))}>
                  Test sign-in
                </MenuItem>
                <MenuItem icon={Power} onSelect={() => toggle(p)}>
                  {p.enabled ? "Turn off" : "Turn on"}
                </MenuItem>
                <MenuSeparator />
                <MenuItem icon={Trash2} danger onSelect={() => setDeleting(p)}>
                  Delete
                </MenuItem>
              </MenuContent>
            </Menu>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={Network}
            title="No identity providers yet"
            description="Add Google, Microsoft Entra ID, GitHub or any OpenID Connect provider so people can sign in with the account they already have."
            action={add}
          />
        </Card>
      )}
      {providers.length ? (
        <p className="text-caption text-fg-3">
          Test sign-in signs you in through the provider in this browser and returns here. Use a private window to test with a different account.
        </p>
      ) : null}
      {editing ? (
        <ProviderDialog key={editing === "new" ? "new" : editing.id} provider={editing === "new" ? null : editing} groups={groups} onClose={() => setEditing(null)} />
      ) : null}
      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (open) return;
          setDeleting(null);
          clearError();
        }}
        error={errorFor("delete")}
        title={`Delete ${deleting?.name ?? "identity provider"}?`}
        description={`${pluralize(deleting?.linkedCount ?? 0, "linked account")} will be unlinked. People who only sign in with ${deleting?.name ?? "it"} can't sign in until they get another method.`}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              Delete provider
            </Button>
          </>
        }
      />
    </div>
  );
}

function ProviderDialog({ provider, groups, onClose }: { provider: IdentityProvider | null; groups: Group[]; onClose: () => void }) {
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
  const [errors, setErrors] = useState<
    Partial<Record<"name" | "issuer" | "tenant" | "clientId" | "clientSecret" | "domains" | "profileMap" | "groupClaim" | "groupMap", string>>
  >({});
  const mapped = kind !== "github";
  const unusedFields = (Object.keys(FIELD) as ProfileField[]).filter((f) => !profileMap.some((m) => m.field === f));
  const assigned = groups.filter((g) => g.kind === "assigned");
  const secretRequired = !provider?.hasSecret && kind !== "oidc";

  function pick(next: ProviderKind) {
    if (name === KIND[kind].name) setName(KIND[next].name);
    if (scopes === KIND[kind].scopes) setScopes(KIND[next].scopes);
    setKind(next);
    setErrors({});
  }

  function save() {
    const next = {
      name: name.trim() ? undefined : "Enter the name people see on the sign-in button.",
      tenant: kind === "microsoft" && !tenant.trim() ? "Enter the directory (tenant) ID from the Entra app's overview page." : undefined,
      issuer: kind === "oidc" && !issuer.trim() ? "Enter the issuer URL, for example https://auth.example.com." : undefined,
      clientId: clientId.trim() ? undefined : "Enter the client ID from the provider's app registration.",
      clientSecret: secretRequired && !clientSecret.trim() ? "Enter the client secret from the provider's app registration." : undefined,
      domains:
        words(domains).length > 0
          ? undefined
          : jit
            ? "Add at least one domain so only people from your organization get an account."
            : routeByDomain
              ? "Add at least one domain to send people straight to this provider."
              : undefined,
      profileMap: mapped && profileMap.some((m) => !m.claim.trim()) ? "Enter the claim for every profile field." : undefined,
      groupClaim: mapped && groupMap.length && !groupClaim.trim() ? "Enter the claim that lists the person's groups." : undefined,
      groupMap: mapped && groupMap.some((m) => !m.value.trim() || !m.groupId) ? "Enter a value and choose a group on every row." : undefined,
    };
    setErrors(next);
    if (Object.values(next).some(Boolean)) return;
    const body = {
      kind,
      name: name.trim(),
      issuer: kind === "microsoft" ? `https://login.microsoftonline.com/${tenant.trim()}/v2.0` : issuer.trim(),
      clientId: clientId.trim(),
      clientSecret: clientSecret.trim(),
      scopes: words(scopes),
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
          title: provider ? `${saved.name} saved` : `${saved.name} added`,
          description: saved.showOnSignIn
            ? "Its button is on the sign-in page."
            : saved.routeByDomain
              ? `People from ${saved.allowedDomains.join(", ")} go straight to it when they sign in.`
              : "Use Test sign-in to check it, then show it on the sign-in page.",
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
      title={provider ? `Edit ${provider.name}` : "Add identity provider"}
      description="Create an app registration at the provider with the callback URL from this page, then copy its client ID and secret here."
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {provider ? "Save provider" : "Add provider"}
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
            <legend className="mb-3 text-label text-fg">Provider</legend>
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
                  <span className="text-caption text-fg-3">{KIND[option].description}</span>
                </label>
              ))}
            </div>
          </fieldset>
        )}
        <Field label="Name" hint="Shown on the sign-in button, as in “Continue with Google”." error={errors.name}>
          {(props) => <Input {...props} autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder="Acme SSO" />}
        </Field>
        {kind === "microsoft" ? (
          <Field label="Directory (tenant) ID" hint="From the Entra app's overview page. Only this tenant's accounts can sign in." error={errors.tenant}>
            {(props) => (
              <Input {...props} mono autoComplete="off" spellCheck={false} value={tenant} onChange={(event) => setTenant(event.target.value)} placeholder="72f988bf-86f1-41af-91ab-2d7cd011db47" />
            )}
          </Field>
        ) : null}
        {kind === "github" ? (
          <Field label="GitHub URL" hint="Leave empty for GitHub.com. Enter your server's address for GitHub Enterprise Server.">
            {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={issuer} onChange={(event) => setIssuer(event.target.value)} placeholder="https://github.com" />}
          </Field>
        ) : null}
        {kind === "oidc" ? (
          <Field label="Issuer URL" hint="Halo reads the provider's settings from /.well-known/openid-configuration under this URL." error={errors.issuer}>
            {(props) => (
              <Input {...props} mono autoComplete="off" spellCheck={false} value={issuer} onChange={(event) => setIssuer(event.target.value)} placeholder="https://auth.example.com" />
            )}
          </Field>
        ) : null}
        <Field label="Client ID" error={errors.clientId}>
          {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={clientId} onChange={(event) => setClientId(event.target.value)} />}
        </Field>
        <Field
          label="Client secret"
          hint={provider?.hasSecret ? "Leave empty to keep the current secret. Halo stores it encrypted and never shows it again." : kind === "oidc" ? "Optional for public clients that use PKCE. Stored encrypted." : "Stored encrypted. Halo never shows it again."}
          error={errors.clientSecret}
        >
          {(props) => (
            <Input {...props} mono type="password" autoComplete="new-password" spellCheck={false} value={clientSecret} onChange={(event) => setClientSecret(event.target.value)} />
          )}
        </Field>
        <Field label="Scopes" hint="Separated by spaces. Halo needs the email address and a stable account ID.">
          {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={scopes} onChange={(event) => setScopes(event.target.value)} />}
        </Field>
        <Field
          label="Allowed email domains"
          hint="Separated by spaces. Only verified addresses at these domains can link to a Halo account. Leave empty to link any verified address that matches an existing account."
          error={errors.domains}
        >
          {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={domains} onChange={(event) => setDomains(event.target.value)} placeholder="example.com" />}
        </Field>
        <fieldset className="flex flex-col gap-4">
          <legend className="mb-3 text-label text-fg">Sign-in</legend>
          <Toggle checked={enabled} onChange={setEnabled} label="Turned on" description="People with a linked account can sign in with it. Test sign-in needs it on." />
          <Toggle
            checked={showOnSignIn}
            onChange={setShowOnSignIn}
            label="Show on the sign-in page"
            description="Leave off to test it from here before everyone sees the button, or to reach it only by email domain."
          />
          <Toggle
            checked={routeByDomain}
            onChange={setRouteByDomain}
            label="Send allowed domains straight to this provider"
            description="People who enter an address at an allowed domain skip the other sign-in options. Works with the button hidden."
          />
          <Toggle
            checked={jit}
            onChange={setJit}
            label="Create accounts on first sign-in"
            description="New people from the allowed domains get an active Halo account. Off means only existing accounts can sign in."
          />
        </fieldset>
        {jit ? (
          <fieldset className="flex animate-enter flex-col gap-3">
            <legend className="mb-3 text-label text-fg">Add new people to</legend>
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
              <p className="text-body-sm text-fg-3">No assigned groups yet. Dynamic groups pick people up from their profile.</p>
            )}
          </fieldset>
        ) : null}
        {mapped ? (
          <>
            <fieldset className="flex flex-col gap-3">
              <legend className="mb-3 text-label text-fg">Profile from claims</legend>
              {profileMap.map((m, index) => (
                <div key={index} className="flex items-center gap-2">
                  <Input
                    mono
                    className="min-w-0"
                    aria-label={`Claim for ${FIELD[m.field]}`}
                    autoComplete="off"
                    spellCheck={false}
                    value={m.claim}
                    onChange={(event) => setProfileMap((current) => current.map((c, i) => (i === index ? { ...c, claim: event.target.value } : c)))}
                    placeholder="department"
                  />
                  <Select
                    aria-label={`Profile field ${index + 1}`}
                    value={m.field}
                    onChange={(event) =>
                      setProfileMap((current) => current.map((c, i) => (i === index ? { ...c, field: event.target.value as ProfileField } : c)))
                    }
                  >
                    {(Object.keys(FIELD) as ProfileField[]).map((f) => (
                      <option key={f} value={f} disabled={f !== m.field && !unusedFields.includes(f)}>
                        {FIELD[f]}
                      </option>
                    ))}
                  </Select>
                  <Button
                    size="icon"
                    variant="quiet"
                    aria-label={`Stop mapping ${FIELD[m.field]}`}
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
                Map a profile field
              </Button>
              {errors.profileMap ? (
                <p className="text-caption text-danger">{errors.profileMap}</p>
              ) : (
                <p className="text-caption text-fg-3">
                  Copied on every sign-in. A claim the provider leaves out keeps the current value; an empty one clears it, except the name.
                </p>
              )}
            </fieldset>
            <fieldset className="flex flex-col gap-3">
              <legend className="mb-3 text-label text-fg">Groups from claims</legend>
              {assigned.length ? (
                <>
                  {groupMap.length ? (
                    <Field label="Groups claim" error={errors.groupClaim}>
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
                        aria-label={`Provider value ${index + 1}`}
                        autoComplete="off"
                        spellCheck={false}
                        value={m.value}
                        onChange={(event) => setGroupMap((current) => current.map((c, i) => (i === index ? { ...c, value: event.target.value } : c)))}
                        placeholder="Group name or ID"
                      />
                      <Select
                        aria-label={`Halo group for value ${index + 1}`}
                        value={m.groupId}
                        onChange={(event) => setGroupMap((current) => current.map((c, i) => (i === index ? { ...c, groupId: event.target.value } : c)))}
                      >
                        <option value="" disabled>
                          Choose a group
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
                        aria-label={`Remove group row ${index + 1}`}
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
                    Map a group
                  </Button>
                  {errors.groupMap ? (
                    <p className="text-caption text-danger">{errors.groupMap}</p>
                  ) : (
                    <p className="text-caption text-fg-3">
                      On every sign-in, people join a group while the claim contains its value and leave it when the value is gone. Groups not listed here are
                      never changed.
                    </p>
                  )}
                </>
              ) : (
                <p className="text-body-sm text-fg-3">No assigned groups yet. Create one to map provider groups to it.</p>
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
