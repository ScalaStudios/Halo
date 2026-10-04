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
import { formatDate, pluralize } from "@/lib/format";
import type { ApiKey, KeyScope, RoleKey, ServiceAccount } from "@/lib/provisioning-types";
import { RoleCheckboxes } from "./service-accounts-table";
import { KEY_STATE, ROLES, SCOPES, ScopeTags, StatusLabel, activeKeys, canProvision, keyState } from "./shared";

const EXPIRY = [
  { value: "30", label: "30 days" },
  { value: "90", label: "90 days" },
  { value: "365", label: "1 year" },
  { value: "never", label: "No expiry" },
];

export function AccountActions({ account, scimUrl }: { account: ServiceAccount; scimUrl: string }) {
  const { pending, run, toast } = useMutation();
  const [creating, setCreating] = useState(false);
  const [disabling, setDisabling] = useState(false);
  const disabled = account.status === "suspended";

  function setDisabled(next: boolean) {
    void run(
      "status",
      () => api(`/service-accounts/${account.id}`, { method: "PATCH", body: { disabled: next } }),
      () => {
        setDisabling(false);
        toast(next ? { title: `${account.name} disabled`, description: "Its API keys stopped working. Recorded in the audit log." } : { title: `${account.name} enabled`, description: "Its API keys work again." });
      },
    );
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button size="sm" variant="primary" onClick={() => setCreating(true)}>
        <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
        Create API key
      </Button>
      <Menu>
        <MenuTrigger asChild>
          <Button size="icon-sm" variant="quiet" aria-label="More actions" loading={pending === "status" && !disabling}>
            <Ellipsis aria-hidden="true" size={16} strokeWidth={1.75} />
          </Button>
        </MenuTrigger>
        <MenuContent>
          <MenuItem
            icon={Copy}
            onSelect={() => {
              void navigator.clipboard?.writeText(account.id);
              toast({ title: "Service account ID copied", description: account.id });
            }}
          >
            Copy service account ID
          </MenuItem>
          <MenuSeparator />
          {disabled ? (
            <MenuItem icon={Power} onSelect={() => setDisabled(false)}>
              Enable service account
            </MenuItem>
          ) : (
            <MenuItem icon={Power} danger onSelect={() => setDisabling(true)}>
              Disable service account
            </MenuItem>
          )}
        </MenuContent>
      </Menu>
      <CreateKeyDialog account={account} scimUrl={scimUrl} open={creating} onOpenChange={setCreating} />
      <Dialog
        open={disabling}
        onOpenChange={setDisabling}
        title={`Disable ${account.name}?`}
        description={`Its ${pluralize(activeKeys(account.keys).length, "active API key")} stop working immediately. Nothing is deleted, and you can enable it again later.`}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDisabling(false)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={pending === "status"} onClick={() => setDisabled(true)}>
              Disable service account
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
  const { pending, run } = useMutation();

  function close() {
    onOpenChange(false);
    setLabel("");
    setScopes([]);
    setExpiry("90");
    setErrors({});
    setIssued(null);
  }

  function submit() {
    const next = {
      label: label.trim() ? undefined : "Enter a label that says where the key is used.",
      scopes: scopes.length ? undefined : "Choose at least one scope.",
    };
    setErrors(next);
    if (next.label || next.scopes) return;
    const expiresAt = expiry === "never" ? null : new Date(Date.now() + Number(expiry) * 86_400_000).toISOString();
    void run("create", () => api<{ key: ApiKey; secret: string }>(`/service-accounts/${account.id}/keys`, { body: { label: label.trim(), scopes, expiresAt } }), setIssued);
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => (next ? onOpenChange(true) : close())}
      title={issued ? "New API key" : "Create API key"}
      description={
        issued
          ? `${account.name} authenticates with this key as Authorization: Bearer <key>. ${issued.key.expiresAt ? `It expires ${formatDate(issued.key.expiresAt)}.` : "It does not expire."}`
          : `The key acts as ${account.name}, with its roles. Halo stores only a hash, so you see the key once.`
      }
      footer={
        issued ? (
          <Button variant="primary" onClick={close}>
            Done
          </Button>
        ) : (
          <>
            <Button variant="secondary" onClick={close}>
              Cancel
            </Button>
            <Button variant="primary" loading={pending === "create"} onClick={submit}>
              Create API key
            </Button>
          </>
        )
      }
    >
      {issued ? (
        <div className="flex animate-enter flex-col gap-4">
          <CopyField label="API key" value={issued.secret} secret />
          {issued.key.scopes.includes("scim") ? <CopyField label="SCIM base URL" value={scimUrl} /> : null}
          <SecretWarning>This is the only time Halo shows this key. Store it in your secret manager before closing this dialog.</SecretWarning>
        </div>
      ) : (
        <form
          className="flex flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label="Label" error={errors.label}>
            {(props) => <Input {...props} autoFocus autoComplete="off" value={label} onChange={(event) => setLabel(event.target.value)} placeholder="Production CI" />}
          </Field>
          <fieldset className="flex flex-col gap-3" aria-describedby={errors.scopes ? "scopes-error" : undefined}>
            <legend className="mb-3 text-label text-fg">Scopes</legend>
            {(Object.keys(SCOPES) as KeyScope[]).map((scope) => (
              <label key={scope} className="flex cursor-pointer items-start gap-3">
                <Checkbox
                  className="mt-0.5"
                  checked={scopes.includes(scope)}
                  onChange={(event) => setScopes((current) => (event.target.checked ? [...current, scope] : current.filter((s) => s !== scope)))}
                />
                <span className="flex flex-col">
                  <span className="text-body-sm text-fg">
                    {SCOPES[scope].label} <span className="font-mono text-code-sm text-fg-3">{scope}</span>
                  </span>
                  <span className="text-caption text-fg-3">{SCOPES[scope].description}</span>
                  {scope === "scim" && scopes.includes("scim") && !canProvision(account) ? (
                    <span className="text-caption text-warning">{account.name} doesn&apos;t have the User administrator role, so SCIM requests are rejected until you add it.</span>
                  ) : null}
                </span>
              </label>
            ))}
            {errors.scopes ? (
              <p id="scopes-error" className="text-caption text-danger">
                {errors.scopes}
              </p>
            ) : null}
          </fieldset>
          <Field label="Expires">
            {(props) => (
              <Select {...props} value={expiry} onChange={(event) => setExpiry(event.target.value)}>
                {EXPIRY.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
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

export function RolesCard({ account }: { account: ServiceAccount }) {
  const [editing, setEditing] = useState(false);
  const [roles, setRoles] = useState<RoleKey[]>(account.roles);
  const { pending, run, toast } = useMutation();

  function save() {
    void run(
      "roles",
      () => api<ServiceAccount>(`/service-accounts/${account.id}/roles`, { method: "PUT", body: { roles } }),
      () => {
        setEditing(false);
        toast({ title: "Roles updated", description: `${account.name} now acts with ${pluralize(roles.length, "role")}. Recorded in the audit log.` });
      },
    );
  }

  return (
    <Card>
      <CardHeader
        title="Roles"
        description="What requests with this service account's keys may change."
        actions={
          <Button
            size="sm"
            variant="secondary"
            onClick={() => {
              setRoles(account.roles);
              setEditing(true);
            }}
          >
            Edit roles
          </Button>
        }
      />
      <CardBody>
        {account.roles.length ? (
          <ul className="flex flex-col gap-3">
            {account.roles.map((role) => (
              <li key={role} className="flex flex-col gap-1 rounded-md border border-border p-4">
                <span className="text-body-sm font-semibold text-fg">{ROLES[role].label}</span>
                <span className="text-caption text-fg-3">{ROLES[role].description}</span>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-body-sm text-fg-3">No roles. Requests with this service account&apos;s keys are rejected until you add one.</p>
        )}
      </CardBody>
      <Dialog
        open={editing}
        onOpenChange={setEditing}
        title={`Edit ${account.name}'s roles`}
        description="You can only give roles you hold yourself. Global administrators can give any role."
        footer={
          <>
            <Button variant="secondary" onClick={() => setEditing(false)}>
              Cancel
            </Button>
            <Button variant="primary" loading={pending === "roles"} onClick={save}>
              Save roles
            </Button>
          </>
        }
      >
        <RoleCheckboxes selected={roles} onChange={setRoles} />
      </Dialog>
    </Card>
  );
}

export function KeyTable({ keys, accountName }: { keys: ApiKey[]; accountName: string }) {
  const [revoking, setRevoking] = useState<ApiKey | null>(null);

  if (keys.length === 0) {
    return <EmptyState icon={KeyRound} title="No API keys" description={`Create a key so ${accountName} can call the Halo API or provision people over SCIM.`} />;
  }

  return (
    <>
      <SimpleTable
        caption="API keys"
        head={["Key", "Scopes", "Status", "Created", "Last used", "Expires", "Actions"]}
        rows={keys.map((key) => {
          const state = keyState(key);
          return [
            <span key="k" className="flex flex-col">
              <span className="font-medium text-fg">{key.label}</span>
              <span className="font-mono text-code-sm text-fg-3">{key.prefix}…</span>
            </span>,
            <ScopeTags key="s" scopes={key.scopes} />,
            <StatusLabel key="t" {...KEY_STATE[state]} />,
            <span key="c" className="tnum whitespace-nowrap">
              {formatDate(key.createdAt)}
            </span>,
            <span key="u" className="whitespace-nowrap">
              {key.lastUsedAt ? <RelativeTime iso={key.lastUsedAt} /> : "Never"}
            </span>,
            state === "revoked" ? "—" : key.expiresAt ? <ExpiryText key="e" iso={key.expiresAt} /> : "Never",
            <span key="a" className="flex justify-end">
              {state === "revoked" ? null : (
                <Button size="sm" variant="secondary" onClick={() => setRevoking(key)}>
                  Revoke
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
  const { pending, run, toast } = useMutation();

  return (
    <Dialog
      open={apiKey !== null}
      onOpenChange={(next) => (next ? undefined : onClose())}
      title={`Revoke “${apiKey?.label ?? ""}”?`}
      description={apiKey ? `Requests that use ${apiKey.prefix}… fail immediately. ${apiKey.serviceAccountName} keeps its other keys. A revoked key can't be restored.` : undefined}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
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
                  toast({ title: `“${apiKey.label}” revoked`, description: "Recorded in the audit log." });
                },
              )
            }
          >
            Revoke key
          </Button>
        </>
      }
    />
  );
}
