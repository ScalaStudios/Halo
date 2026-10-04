"use client";

import Link from "next/link";
import { AppWindow, Pencil, Plus, Trash2, X } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Tag } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Checkbox, Field, Input, Select, Textarea } from "@/components/ui/input";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { formatDuration, pluralize } from "@/lib/format";
import type { ApiResource, ApiScope } from "@/lib/infra-types";
import type { Application } from "@/lib/types";

const LIFETIMES = [300, 900, 3600, 14400, 28800, 86400];

type GrantTarget = Pick<Application, "id" | "name">;

export function NewResourceButton() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button size="sm" variant="primary" onClick={() => setOpen(true)}>
        <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
        Add API
      </Button>
      {open ? <ResourceDialog resource={null} onClose={() => setOpen(false)} /> : null}
    </>
  );
}

export function ResourceActions({ resource }: { resource: ApiResource }) {
  const { pending, run, toast, router } = useMutation();
  const [editing, setEditing] = useState(false);
  const [deleting, setDeleting] = useState(false);

  function remove() {
    void run("delete", () => api(`/api-resources/${resource.id}`, { method: "DELETE" }), () => {
      toast({ title: `${resource.name} deleted`, description: `${pluralize(resource.grants.length, "application")} lost access. Recorded in the audit log.` });
      router.push("/admin/api-resources");
    });
  }

  return (
    <div className="flex flex-wrap gap-2">
      <Button size="sm" variant="secondary" onClick={() => setEditing(true)}>
        <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
        Edit API
      </Button>
      <Button size="sm" variant="secondary" onClick={() => setDeleting(true)}>
        <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
        Delete
      </Button>
      {editing ? <ResourceDialog resource={resource} onClose={() => setEditing(false)} /> : null}
      <Dialog
        open={deleting}
        onOpenChange={setDeleting}
        title={`Delete ${resource.name}?`}
        description={`Halo stops issuing tokens for ${resource.identifier} and removes its grants to ${pluralize(resource.grants.length, "application")}. Tokens already issued stay valid until they expire.`}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(false)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              Delete API
            </Button>
          </>
        }
      />
    </div>
  );
}

function ResourceDialog({ resource, onClose }: { resource: ApiResource | null; onClose: () => void }) {
  const { pending, run, toast, router } = useMutation();
  const [name, setName] = useState(resource?.name ?? "");
  const [identifier, setIdentifier] = useState(resource?.identifier ?? "");
  const [description, setDescription] = useState(resource?.description ?? "");
  const [ttl, setTtl] = useState(resource?.accessTokenTtl ?? 900);
  const [scopes, setScopes] = useState<ApiScope[]>(resource?.scopes ?? [{ name: "", description: "" }]);
  const [errors, setErrors] = useState<{ name?: string; identifier?: string; scopes?: string }>({});
  const lifetimes = LIFETIMES.includes(ttl) ? LIFETIMES : [...LIFETIMES, ttl].sort((a, b) => a - b);

  function update(index: number, change: Partial<ApiScope>) {
    setScopes((current) => current.map((scope, i) => (i === index ? { ...scope, ...change } : scope)));
  }

  function save() {
    const named = scopes.map((s) => ({ name: s.name.trim(), description: s.description.trim() })).filter((s) => s.name);
    const next = {
      name: name.trim() ? undefined : "Enter a name, for example “Billing API”.",
      identifier: identifier.trim() ? undefined : "Enter the identifier the API checks in the aud claim, for example https://billing.example.com.",
      scopes: named.length ? undefined : "Add at least one scope, such as billing:read.",
    };
    setErrors(next);
    if (next.name || next.identifier || next.scopes) return;
    const body = { name: name.trim(), identifier: identifier.trim(), description: description.trim(), accessTokenTtl: ttl, scopes: named };
    void run(
      "save",
      () => (resource ? api<ApiResource>(`/api-resources/${resource.id}`, { method: "PUT", body }) : api<ApiResource>("/api-resources", { body })),
      (saved) => {
        toast({ title: resource ? `${saved.name} saved` : `${saved.name} added`, description: `${pluralize(saved.scopes.length, "scope")}. Recorded in the audit log.` });
        onClose();
        if (!resource) router.push(`/admin/api-resources/${saved.id}`);
      },
    );
  }

  return (
    <Dialog
      open
      width="wide"
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={resource ? `Edit ${resource.name}` : "Add API"}
      description="An API is a resource server that accepts Halo access tokens. Applications you grant its scopes get tokens addressed to it."
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {resource ? "Save API" : "Add API"}
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
        <div className="grid grid-cols-1 gap-6 sm:grid-cols-2">
          <Field label="Name" error={errors.name}>
            {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder="Billing API" />}
          </Field>
          <Field label="Access token lifetime" hint="Overrides the application's lifetime for tokens addressed to this API.">
            {(props) => (
              <Select {...props} value={ttl} onChange={(event) => setTtl(Number(event.target.value))}>
                {lifetimes.map((seconds) => (
                  <option key={seconds} value={seconds}>
                    {formatDuration(seconds)}
                  </option>
                ))}
              </Select>
            )}
          </Field>
        </div>
        <Field label="Identifier" error={errors.identifier} hint="An absolute URI. Tokens carry it in the aud claim, and the API rejects tokens without it.">
          {(props) => (
            <Input {...props} mono autoComplete="off" spellCheck={false} value={identifier} onChange={(event) => setIdentifier(event.target.value)} placeholder="https://billing.example.com" />
          )}
        </Field>
        <Field label="Description">
          {(props) => <Textarea {...props} rows={2} value={description} onChange={(event) => setDescription(event.target.value)} placeholder="What the API does and who runs it." />}
        </Field>
        <fieldset className="flex flex-col gap-3" aria-describedby={errors.scopes ? "scopes-error" : undefined}>
          <legend className="mb-1 text-label text-fg">Scopes</legend>
          <p className="text-caption text-fg-3">Names are unique across every API, so prefix them with the API, such as billing:read.</p>
          {scopes.map((scope, index) => (
            <div key={index} className="flex items-start gap-2">
              <Input
                mono
                aria-label={`Scope ${index + 1} name`}
                autoComplete="off"
                spellCheck={false}
                value={scope.name}
                onChange={(event) => update(index, { name: event.target.value })}
                placeholder="billing:read"
                className="sm:w-56"
              />
              <Input
                aria-label={`Scope ${index + 1} description`}
                autoComplete="off"
                value={scope.description}
                onChange={(event) => update(index, { description: event.target.value })}
                placeholder="Read invoices and usage records."
              />
              <Button
                size="icon"
                variant="quiet"
                aria-label={`Remove scope ${scope.name || index + 1}`}
                disabled={scopes.length === 1}
                onClick={() => setScopes((current) => current.filter((_, i) => i !== index))}
              >
                <X aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
            </div>
          ))}
          {errors.scopes ? (
            <p id="scopes-error" className="text-caption text-danger">
              {errors.scopes}
            </p>
          ) : null}
          <Button size="sm" variant="secondary" className="self-start" onClick={() => setScopes((current) => [...current, { name: "", description: "" }])}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            Add scope
          </Button>
        </fieldset>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

export function GrantsSection({ resource, applications }: { resource: ApiResource; applications: GrantTarget[] }) {
  const { pending, run, toast } = useMutation();
  const [editing, setEditing] = useState<{ appId: string; scopes: string[] } | "new" | null>(null);
  const [removing, setRemoving] = useState<GrantTarget | null>(null);
  const appName = (id: string) => applications.find((a) => a.id === id)?.name ?? "Deleted application";
  const ungranted = applications.filter((a) => !resource.grants.some((g) => g.appId === a.id));

  function revoke() {
    const app = removing;
    if (!app) return;
    void run("remove", () => api(`/api-resources/${resource.id}/grants/${app.id}`, { method: "PUT", body: { scopes: [] } }), () => {
      setRemoving(null);
      toast({ title: `${app.name} no longer has access to ${resource.name}`, description: "Its next token leaves out these scopes. Tokens already issued stay valid until they expire." });
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <SectionTitle
        title="Applications"
        description="Applications granted scopes get JWT access tokens with this API's identifier in aud when they request a granted scope or send resource=<identifier>."
        action={
          <Button size="sm" variant="primary" disabled={ungranted.length === 0} onClick={() => setEditing("new")}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            Grant access
          </Button>
        }
      />
      {resource.grants.length ? (
        <SimpleTable
          caption={`Applications with access to ${resource.name}`}
          head={["Application", "Granted scopes", "Actions"]}
          rows={resource.grants.map((grant) => [
            <Link key="app" href={`/admin/applications/${grant.appId}`} className="font-medium whitespace-nowrap text-fg underline-offset-3 hover:underline">
              {appName(grant.appId)}
            </Link>,
            <span key="scopes" className="flex flex-wrap gap-1">
              {grant.scopes.map((scope) => (
                <Tag key={scope}>{scope}</Tag>
              ))}
            </span>,
            <span key="actions" className="flex gap-1">
              <Button size="icon-sm" variant="quiet" aria-label={`Edit the scopes of ${appName(grant.appId)}`} onClick={() => setEditing(grant)}>
                <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
              <Button size="icon-sm" variant="quiet" aria-label={`Remove access for ${appName(grant.appId)}`} onClick={() => setRemoving({ id: grant.appId, name: appName(grant.appId) })}>
                <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
            </span>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={AppWindow}
            title="No application has access yet"
            description="Grant scopes to the applications that call this API. Until then, Halo issues no token addressed to it."
          />
        </Card>
      )}
      {editing ? (
        <GrantDialog
          key={editing === "new" ? "new" : editing.appId}
          resource={resource}
          grant={editing === "new" ? null : editing}
          applications={editing === "new" ? ungranted : applications.filter((a) => a.id === editing.appId)}
          onClose={() => setEditing(null)}
        />
      ) : null}
      <Dialog
        open={removing !== null}
        onOpenChange={(open) => {
          if (!open) setRemoving(null);
        }}
        title={`Remove access for ${removing?.name ?? "this application"}?`}
        description="Its next token leaves out every scope of this API. Tokens already issued stay valid until they expire."
        footer={
          <>
            <Button variant="secondary" onClick={() => setRemoving(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={pending === "remove"} onClick={revoke}>
              Remove access
            </Button>
          </>
        }
      />
    </div>
  );
}

function GrantDialog({
  resource,
  grant,
  applications,
  onClose,
}: {
  resource: ApiResource;
  grant: { appId: string; scopes: string[] } | null;
  applications: GrantTarget[];
  onClose: () => void;
}) {
  const { pending, run, toast } = useMutation();
  const [appId, setAppId] = useState(grant?.appId ?? "");
  const [scopes, setScopes] = useState<string[]>(grant?.scopes ?? []);
  const [errors, setErrors] = useState<{ app?: string; scopes?: string }>({});

  function save() {
    const next = {
      app: appId ? undefined : "Choose the application that calls this API.",
      scopes: scopes.length ? undefined : "Choose at least one scope. To take every scope away, remove the grant instead.",
    };
    setErrors(next);
    if (next.app || next.scopes) return;
    void run("save", () => api<ApiResource>(`/api-resources/${resource.id}/grants/${appId}`, { method: "PUT", body: { scopes } }), () => {
      const name = applications.find((a) => a.id === appId)?.name ?? "The application";
      toast({ title: `${name} can use ${scopes.join(", ")}`, description: "Applies to the next token it requests. Recorded in the audit log." });
      onClose();
    });
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={grant ? `Edit access for ${applications[0]?.name ?? "application"}` : `Grant access to ${resource.name}`}
      description="The application receives only the scopes you check here, and only when it asks for them."
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {grant ? "Save access" : "Grant access"}
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
        <Field label="Application" error={errors.app}>
          {(props) => (
            <Select {...props} value={appId} disabled={grant !== null} onChange={(event) => setAppId(event.target.value)}>
              <option value="" disabled>
                Choose an application
              </option>
              {applications.map((a) => (
                <option key={a.id} value={a.id}>
                  {a.name}
                </option>
              ))}
            </Select>
          )}
        </Field>
        <fieldset className="flex flex-col gap-3" aria-describedby={errors.scopes ? "grant-scopes-error" : undefined}>
          <legend className="mb-3 text-label text-fg">Scopes</legend>
          {resource.scopes.map((scope) => (
            <label key={scope.name} className="flex cursor-pointer items-start gap-3">
              <Checkbox
                className="mt-0.5"
                checked={scopes.includes(scope.name)}
                onChange={(event) => setScopes((current) => (event.target.checked ? [...current, scope.name] : current.filter((s) => s !== scope.name)))}
              />
              <span className="flex flex-col">
                <span className="font-mono text-code-sm text-fg">{scope.name}</span>
                {scope.description ? <span className="text-caption text-fg-3">{scope.description}</span> : null}
              </span>
            </label>
          ))}
          {errors.scopes ? (
            <p id="grant-scopes-error" className="text-caption text-danger">
              {errors.scopes}
            </p>
          ) : null}
        </fieldset>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
