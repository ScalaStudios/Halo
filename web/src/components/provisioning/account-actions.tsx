"use client";

import { Copy, Ellipsis, KeyRound, Plus, Power } from "lucide-react";
import { useState } from "react";
import { ExpiryText, SecretWarning } from "@/components/applications/shared";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Checkbox, Field, Input, Select } from "@/components/ui/input";
import { Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger } from "@/components/ui/menu";
import { RelativeTime } from "@/components/ui/relative-time";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { formatDate } from "@/lib/format";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { provisioningMessages } from "@/lib/i18n/console/infrastructure/provisioning";
import type { ApiKey, KeyScope, RoleKey, ServiceAccount } from "@/lib/provisioning-types";
import { RoleCheckboxes } from "./service-accounts-table";
import { KEY_STATE, RateLimitHits, ScopeTags, StatusLabel, activeKeys, canProvision, keyState } from "./shared";

const EXPIRY = ["30", "90", "365", "never"];

export function AccountActions({ account, scimUrl }: { account: ServiceAccount; scimUrl: string }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [creating, setCreating] = useState(false);
  const [disabling, setDisabling] = useState(false);
  const t = useMessages(provisioningMessages);
  const disabled = account.status === "suspended";

  function setDisabled(next: boolean) {
    void run(
      "status",
      () => api(`/service-accounts/${account.id}`, { method: "PATCH", body: { disabled: next } }),
      () => {
        setDisabling(false);
        toast(next ? { title: t.account.disabled(account.name), description: t.account.disabledHint } : { title: t.account.enabled(account.name), description: t.account.enabledHint });
      },
      next ? "inline" : undefined,
    );
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
        <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
        {t.account.createKey}
      </Button>
      <Menu>
        <MenuTrigger asChild>
          <Button size="icon-sm" variant="quiet" aria-label={t.account.moreActions} loading={pending === "status" && !disabling}>
            <Ellipsis aria-hidden="true" size={16} strokeWidth={1.75} />
          </Button>
        </MenuTrigger>
        <MenuContent>
          <MenuItem
            icon={Copy}
            onSelect={() => {
              void navigator.clipboard?.writeText(account.id);
              toast({ title: t.serviceAccounts.idCopied, description: account.id });
            }}
          >
            {t.serviceAccounts.copyId}
          </MenuItem>
          <MenuSeparator />
          {disabled ? (
            <MenuItem icon={Power} onSelect={() => setDisabled(false)}>
              {t.account.enable}
            </MenuItem>
          ) : (
            <MenuItem icon={Power} danger onSelect={() => setDisabling(true)}>
              {t.account.disable}
            </MenuItem>
          )}
        </MenuContent>
      </Menu>
      <CreateKeyDialog account={account} scimUrl={scimUrl} open={creating} onOpenChange={setCreating} />
      <Dialog
        open={disabling}
        onOpenChange={(next) => {
          clearError();
          setDisabling(next);
        }}
        error={errorFor("status")}
        title={t.account.disableTitle(account.name)}
        description={t.account.disableHint(activeKeys(account.keys).length)}
        footer={
          <>
            <Button
              variant="secondary"
              onClick={() => {
                clearError();
                setDisabling(false);
              }}
            >
              {t.cancel}
            </Button>
            <Button variant="destructive" loading={pending === "status"} onClick={() => setDisabled(true)}>
              {t.account.disable}
            </Button>
          </>
        }
      />
    </div>
  );
}

function CreateKeyDialog({ account, scimUrl, open, onOpenChange }: { account: ServiceAccount; scimUrl: string; open: boolean; onOpenChange: (open: boolean) => void }) {
  const [label, setLabel] = useState("");
  const [scopes, setScopes] = useState<KeyScope[]>([]);
  const [expiry, setExpiry] = useState("90");
  const [errors, setErrors] = useState<{ label?: string; scopes?: string }>({});
  const [issued, setIssued] = useState<{ key: ApiKey; secret: string } | null>(null);
  const { pending, run, errorFor, clearError } = useMutation();
  const locale = useLocale();
  const t = provisioningMessages[locale];

  function close() {
    onOpenChange(false);
    clearError();
    setLabel("");
    setScopes([]);
    setExpiry("90");
    setErrors({});
    setIssued(null);
  }

  function submit() {
    const next = {
      label: label.trim() ? undefined : t.account.labelError,
      scopes: scopes.length ? undefined : t.account.scopesError,
    };
    setErrors(next);
    if (next.label || next.scopes) return;
    const expiresAt = expiry === "never" ? null : new Date(Date.now() + Number(expiry) * 86_400_000).toISOString();
    void run("create", () => api<{ key: ApiKey; secret: string }>(`/service-accounts/${account.id}/keys`, { body: { label: label.trim(), scopes, expiresAt } }), setIssued, "inline");
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => (next ? onOpenChange(true) : close())}
      error={errorFor("create")}
      title={issued ? t.account.newKey : t.account.createKey}
      description={issued ? t.account.issuedHint(account.name, issued.key.expiresAt ? formatDate(issued.key.expiresAt, locale) : null) : t.account.createHint(account.name)}
      footer={
        issued ? (
          <Button variant="primary" onClick={close}>
            {t.done}
          </Button>
        ) : (
          <>
            <Button variant="secondary" onClick={close}>
              {t.cancel}
            </Button>
            <Button variant="primary" loading={pending === "create"} onClick={submit}>
              {t.account.createKey}
            </Button>
          </>
        )
      }
    >
      {issued ? (
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label={t.account.apiKey} value={issued.secret} secret />
          {issued.key.scopes.includes("scim") ? <CopyField label={t.page.scimBaseUrl} value={scimUrl} /> : null}
          <SecretWarning>{t.account.onlyOnce}</SecretWarning>
        </div>
      ) : (
        <form
          className="flex flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label={t.account.label} error={errors.label}>
            {(props) => <Input {...props} autoFocus autoComplete="off" value={label} onChange={(event) => setLabel(event.target.value)} placeholder={t.account.labelPlaceholder} />}
          </Field>
          <ScopeFieldset account={account} scopes={scopes} onChange={setScopes} error={errors.scopes} />
          <Field label={t.apiKeys.expires}>
            {(props) => (
              <Select {...props} value={expiry} onChange={(event) => setExpiry(event.target.value)}>
                {EXPIRY.map((value) => (
                  <option key={value} value={value}>
                    {t.account.expiry[value]}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <button type="submit" hidden />
        </form>
      )}
    </Dialog>
  );
}

export function ScopeFieldset({ account, scopes, onChange, error }: { account: ServiceAccount; scopes: KeyScope[]; onChange: (scopes: KeyScope[]) => void; error?: string }) {
  const t = useMessages(provisioningMessages);
  return (
    <fieldset className="flex flex-col gap-3" aria-describedby={error ? "scopes-error" : undefined}>
      <legend className="mb-3 text-label text-fg">{t.scopes}</legend>
      {(Object.keys(t.scope) as KeyScope[]).map((scope) => (
        <label key={scope} className="flex cursor-pointer items-start gap-3">
          <Checkbox className="mt-0.5" checked={scopes.includes(scope)} onChange={(event) => onChange(event.target.checked ? [...scopes, scope] : scopes.filter((s) => s !== scope))} />
          <span className="flex flex-col">
            <span className="text-body-sm text-fg">
              {t.scope[scope].label} <span className="font-mono text-code-sm text-fg-3">{scope}</span>
            </span>
            <span className="text-caption text-fg-3">{t.scope[scope].description}</span>
            {scope === "scim" && scopes.includes("scim") && !canProvision(account) ? (
              <span className="text-caption text-warning">{t.account.missingRole(account.name)}</span>
            ) : null}
          </span>
        </label>
      ))}
      {error ? (
        <p id="scopes-error" className="text-caption text-danger">
          {error}
        </p>
      ) : null}
    </fieldset>
  );
}

export function RolesCard({ account }: { account: ServiceAccount }) {
  const [editing, setEditing] = useState(false);
  const [roles, setRoles] = useState<RoleKey[]>(account.roles);
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const t = useMessages(provisioningMessages);

  function save() {
    void run(
      "roles",
      () => api<ServiceAccount>(`/service-accounts/${account.id}/roles`, { method: "PUT", body: { roles } }),
      () => {
        setEditing(false);
        toast({ title: t.account.rolesUpdated, description: t.account.rolesUpdatedHint(account.name, roles.length) });
      },
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader
        title={t.roles}
        description={t.account.rolesHint}
        actions={
          <Button
            size="sm"
            variant="secondary"
            onClick={() => {
              setRoles(account.roles);
              setEditing(true);
            }}
          >
            {t.account.editRoles}
          </Button>
        }
      />
      <CardBody>
        {account.roles.length ? (
          <ul className="flex flex-col gap-3">
            {account.roles.map((role) => (
              <li key={role} className="flex flex-col gap-1 rounded-md border border-border p-4">
                <span className="text-body-sm font-semibold text-fg">{t.role[role].label}</span>
                <span className="text-caption text-fg-3">{t.role[role].description}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-body-sm text-fg-3">{t.account.noRoles}</p>
        )}
      </CardBody>
      <Dialog
        open={editing}
        onOpenChange={(next) => {
          clearError();
          setEditing(next);
        }}
        error={errorFor("roles")}
        title={t.account.editRolesTitle(account.name)}
        description={t.account.editRolesHint}
        footer={
          <>
            <Button variant="secondary" onClick={() => setEditing(false)}>
              {t.cancel}
            </Button>
            <Button variant="primary" loading={pending === "roles"} onClick={save}>
              {t.account.saveRoles}
            </Button>
          </>
        }
      >
        <RoleCheckboxes selected={roles} onChange={setRoles} />
      </Dialog>
    </Card>
  );
}

export function RateLimitCard({ account }: { account: ServiceAccount }) {
  const [editing, setEditing] = useState(false);
  const [limit, setLimit] = useState(String(account.rateLimit));
  const [invalid, setInvalid] = useState<string>();
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const locale = useLocale();
  const t = provisioningMessages[locale];

  function close() {
    clearError();
    setInvalid(undefined);
    setEditing(false);
  }

  function save() {
    const next = Number(limit);
    if (!Number.isInteger(next) || next < 1 || next > 100_000) {
      setInvalid(t.account.limitError);
      return;
    }
    setInvalid(undefined);
    void run(
      "rate-limit",
      () => api<ServiceAccount>(`/service-accounts/${account.id}`, { method: "PATCH", body: { rateLimit: next } }),
      () => {
        close();
        toast({ title: t.account.limitUpdated, description: t.account.limitUpdatedHint(account.name, next) });
      },
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader
        title={t.account.rateLimit}
        description={t.account.rateLimitHint}
        actions={
          <Button
            size="sm"
            variant="secondary"
            onClick={() => {
              setLimit(String(account.rateLimit));
              setEditing(true);
            }}
          >
            {t.account.editRateLimit}
          </Button>
        }
      />
      <CardBody className="flex flex-col gap-1">
        <span className="tnum text-body-sm font-semibold text-fg">{t.account.perKey(account.rateLimit.toLocaleString(locale))}</span>
        <span className="text-caption text-fg-3">{t.account.scimLimit}</span>
      </CardBody>
      <Dialog
        open={editing}
        onOpenChange={(next) => (next ? setEditing(true) : close())}
        error={errorFor("rate-limit")}
        title={t.account.editRateLimitTitle(account.name)}
        description={t.account.editRateLimitHint}
        footer={
          <>
            <Button variant="secondary" onClick={close}>
              {t.cancel}
            </Button>
            <Button variant="primary" loading={pending === "rate-limit"} onClick={save}>
              {t.account.saveRateLimit}
            </Button>
          </>
        }
      >
        <form
          noValidate
          onSubmit={(event) => {
            event.preventDefault();
            save();
          }}
        >
          <Field label={t.account.limitField} hint={t.account.limitFieldHint} error={invalid}>
            {(props) => (
              <div className="w-32">
                <Input {...props} autoFocus type="number" inputMode="numeric" min={1} max={100_000} step={1} className="tnum" value={limit} onChange={(event) => setLimit(event.target.value)} />
              </div>
            )}
          </Field>
          <button type="submit" hidden />
        </form>
      </Dialog>
    </Card>
  );
}

export function KeyTable({ keys, accountName }: { keys: ApiKey[]; accountName: string }) {
  const [revoking, setRevoking] = useState<ApiKey | null>(null);
  const locale = useLocale();
  const t = provisioningMessages[locale];

  if (keys.length === 0) {
    return <EmptyState icon={KeyRound} title={t.account.noKeys} description={t.account.noKeysHint(accountName)} />;
  }

  return (
    <>
      <SimpleTable
        caption={t.apiKeys.label}
        head={[t.apiKeys.key, t.scopes, t.status, t.created, t.lastUsed, t.apiKeys.rateLimited, t.apiKeys.expires, t.actions]}
        rows={keys.map((key) => {
          const state = keyState(key);
          return [
            <span key="k" className="flex flex-col">
              <span className="font-medium text-fg">{key.label}</span>
              <span className="font-mono text-code-sm text-fg-3">{key.prefix}…</span>
            </span>,
            <ScopeTags key="s" scopes={key.scopes} />,
            <StatusLabel key="t" tone={KEY_STATE[state]} label={t.keyState[state]} />,
            <span key="c" className="tnum whitespace-nowrap">
              {formatDate(key.createdAt, locale)}
            </span>,
            <span key="u" className="whitespace-nowrap">
              {key.lastUsedAt ? <RelativeTime iso={key.lastUsedAt} /> : t.never}
            </span>,
            <RateLimitHits key="r" hits={key.rateLimitHits} locale={locale} />,
            state === "revoked" ? "—" : key.expiresAt ? <ExpiryText key="e" iso={key.expiresAt} /> : t.never,
            <span key="a" className="flex justify-end">
              {state === "revoked" ? null : (
                <Button size="sm" variant="secondary" onClick={() => setRevoking(key)}>
                  {t.account.revoke}
                </Button>
              )}
            </span>,
          ];
        })}
      />
      <RevokeKeyDialog apiKey={revoking} onClose={() => setRevoking(null)} />
    </>
  );
}

export function RevokeKeyDialog({ apiKey, onClose }: { apiKey: ApiKey | null; onClose: () => void }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const t = useMessages(provisioningMessages);

  function close() {
    clearError();
    onClose();
  }

  return (
    <Dialog
      open={apiKey !== null}
      onOpenChange={(next) => (next ? undefined : close())}
      error={errorFor("revoke")}
      title={t.account.revokeTitle(apiKey?.label ?? "")}
      description={apiKey ? t.account.revokeHint(apiKey.prefix, apiKey.serviceAccountName) : undefined}
      footer={
        <>
          <Button variant="secondary" onClick={close}>
            {t.cancel}
          </Button>
          <Button
            variant="destructive"
            loading={pending === "revoke"}
            onClick={() =>
              apiKey &&
              run(
                "revoke",
                () => api(`/api-keys/${apiKey.id}`, { method: "DELETE" }),
                () => {
                  onClose();
                  toast({ title: t.account.revoked(apiKey.label), description: t.account.revokedHint });
                },
                "inline",
              )
            }
          >
            {t.apiKeys.revoke}
          </Button>
        </>
      }
    />
  );
}
