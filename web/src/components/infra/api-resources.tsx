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
import { useMessages } from "@/lib/i18n/client";
import { infrastructureMessages } from "@/lib/i18n/console/infrastructure";
import type { ApiResource, ApiScope } from "@/lib/infra-types";
import { SIGNING_ALGORITHMS, type Application, type SigningAlgorithm } from "@/lib/types";

const LIFETIMES = [300, 900, 3600, 14400, 28800, 86400];

type GrantTarget = Pick<Application, "id" | "name">;

export function NewResourceButton() {
  const [open, setOpen] = useState(false);
  const t = useMessages(infrastructureMessages).apiResources;
  return (
    <>
      <Button size="sm" variant="primary" onClick={() => setOpen(true)}>
        <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
        {t.add}
      </Button>
      {open ? <ResourceDialog resource={null} onClose={() => setOpen(false)} /> : null}
    </>
  );
}

export function ResourceActions({ resource }: { resource: ApiResource }) {
  const { pending, run, errorFor, clearError, toast, router } = useMutation();
  const [editing, setEditing] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const messages = useMessages(infrastructureMessages);
  const t = messages.apiResources;

  function remove() {
    void run("delete", () => api(`/api-resources/${resource.id}`, { method: "DELETE" }), () => {
      toast({ title: messages.deleted(resource.name), description: t.deletedHint(resource.grants.length) });
      router.push("/admin/api-resources");
    }, "inline");
  }

  return (
    <div className="flex flex-wrap gap-2">
      <Button size="sm" variant="secondary" onClick={() => setEditing(true)}>
        <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
        {t.edit}
      </Button>
      <Button size="sm" variant="secondary" onClick={() => setDeleting(true)}>
        <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
        {t.delete}
      </Button>
      {editing ? <ResourceDialog resource={resource} onClose={() => setEditing(false)} /> : null}
      <Dialog
        open={deleting}
        onOpenChange={(next) => {
          clearError();
          setDeleting(next);
        }}
        error={errorFor("delete")}
        title={t.deleteTitle(resource.name)}
        description={t.deleteHint(resource.identifier, resource.grants.length)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(false)}>
              {messages.cancel}
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              {t.deleteConfirm}
            </Button>
          </>
        }
      />
    </div>
  );
}

function ResourceDialog({ resource, onClose }: { resource: ApiResource | null; onClose: () => void }) {
  const { pending, run, errorFor, toast, router } = useMutation();
  const [name, setName] = useState(resource?.name ?? "");
  const [identifier, setIdentifier] = useState(resource?.identifier ?? "");
  const [description, setDescription] = useState(resource?.description ?? "");
  const [ttl, setTtl] = useState(resource?.accessTokenTtl ?? 900);
  const [alg, setAlg] = useState<SigningAlgorithm>(resource?.accessTokenAlg ?? "RS256");
  const [scopes, setScopes] = useState<ApiScope[]>(resource?.scopes ?? [{ name: "", description: "" }]);
  const [errors, setErrors] = useState<{ name?: string; identifier?: string; scopes?: string }>({});
  const lifetimes = LIFETIMES.includes(ttl) ? LIFETIMES : [...LIFETIMES, ttl].sort((a, b) => a - b);
  const messages = useMessages(infrastructureMessages);
  const t = messages.apiResources;

  function update(index: number, change: Partial<ApiScope>) {
    setScopes((current) => current.map((scope, i) => (i === index ? { ...scope, ...change } : scope)));
  }

  function save() {
    const named = scopes.map((s) => ({ name: s.name.trim(), description: s.description.trim() })).filter((s) => s.name);
    const next = {
      name: name.trim() ? undefined : t.nameError,
      identifier: identifier.trim() ? undefined : t.identifierError,
      scopes: named.length ? undefined : t.scopesError,
    };
    setErrors(next);
    if (next.name || next.identifier || next.scopes) return;
    const body = { name: name.trim(), identifier: identifier.trim(), description: description.trim(), accessTokenTtl: ttl, accessTokenAlg: alg, scopes: named };
    void run(
      "save",
      () => (resource ? api<ApiResource>(`/api-resources/${resource.id}`, { method: "PUT", body }) : api<ApiResource>("/api-resources", { body })),
      (saved) => {
        toast({ title: resource ? messages.saved(saved.name) : t.added(saved.name), description: t.savedHint(saved.scopes.length) });
        onClose();
        if (!resource) router.push(`/admin/api-resources/${saved.id}`);
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      width="wide"
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      error={errorFor("save")}
      title={resource ? messages.edit(resource.name) : t.add}
      description={t.dialogHint}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {messages.cancel}
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {resource ? t.save : t.add}
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
          <Field label={messages.name} error={errors.name}>
            {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder={t.namePlaceholder} />}
          </Field>
          <Field label={t.lifetime} hint={t.lifetimeHint}>
            {(props) => (
              <Select {...props} value={ttl} onChange={(event) => setTtl(Number(event.target.value))}>
                {lifetimes.map((seconds) => (
                  <option key={seconds} value={seconds}>
                    {messages.duration(seconds)}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label={t.signing} hint={alg === "RS256" ? t.rs256Hint : t.algHint(alg)}>
            {(props) => (
              <Select {...props} value={alg} onChange={(event) => setAlg(event.target.value as SigningAlgorithm)}>
                {SIGNING_ALGORITHMS.map(([value, label]) => (
                  <option key={value} value={value}>
                    {label}
                  </option>
                ))}
              </Select>
            )}
          </Field>
        </div>
        <Field label={t.identifier} error={errors.identifier} hint={t.identifierHint}>
          {(props) => (
            <Input {...props} mono autoComplete="off" spellCheck={false} value={identifier} onChange={(event) => setIdentifier(event.target.value)} placeholder="https://billing.example.com" />
          )}
        </Field>
        <Field label={messages.description}>
          {(props) => <Textarea {...props} rows={2} value={description} onChange={(event) => setDescription(event.target.value)} placeholder={t.descriptionPlaceholder} />}
        </Field>
        <fieldset className="flex flex-col gap-3" aria-describedby={errors.scopes ? "scopes-error" : undefined}>
          <legend className="mb-1 text-label text-fg">{t.scopes}</legend>
          <p className="text-caption text-fg-3">{t.scopesHint}</p>
          {scopes.map((scope, index) => (
            <div key={index} className="flex items-start gap-2">
              <Input
                mono
                aria-label={t.scopeName(index + 1)}
                autoComplete="off"
                spellCheck={false}
                value={scope.name}
                onChange={(event) => update(index, { name: event.target.value })}
                placeholder="billing:read"
                className="sm:w-56"
              />
              <Input
                aria-label={t.scopeDescription(index + 1)}
                autoComplete="off"
                value={scope.description}
                onChange={(event) => update(index, { description: event.target.value })}
                placeholder={t.scopeDescriptionPlaceholder}
              />
              <Button
                size="icon"
                variant="quiet"
                aria-label={t.removeScope(scope.name || index + 1)}
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
            {t.addScope}
          </Button>
        </fieldset>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}

export function GrantsSection({ resource, applications }: { resource: ApiResource; applications: GrantTarget[] }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [editing, setEditing] = useState<{ appId: string; scopes: string[] } | "new" | null>(null);
  const [removing, setRemoving] = useState<GrantTarget | null>(null);
  const messages = useMessages(infrastructureMessages);
  const t = messages.apiResources;
  const appName = (id: string) => applications.find((a) => a.id === id)?.name ?? t.deletedApplication;
  const ungranted = applications.filter((a) => !resource.grants.some((g) => g.appId === a.id));

  function revoke() {
    const app = removing;
    if (!app) return;
    void run("remove", () => api(`/api-resources/${resource.id}/grants/${app.id}`, { method: "PUT", body: { scopes: [] } }), () => {
      setRemoving(null);
      toast({ title: t.revokedToast(app.name, resource.name), description: t.revokedHint });
    }, "inline");
  }

  return (
    <div className="flex flex-col gap-6">
      <SectionTitle
        title={t.applications}
        description={t.applicationsHint}
        action={
          <Button size="sm" variant="primary" disabled={ungranted.length === 0} onClick={() => setEditing("new")}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            {t.grant}
          </Button>
        }
      />
      {resource.grants.length ? (
        <SimpleTable
          caption={t.caption(resource.name)}
          head={t.head}
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
              <Button size="icon-sm" variant="quiet" aria-label={t.editScopes(appName(grant.appId))} onClick={() => setEditing(grant)}>
                <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
              <Button size="icon-sm" variant="quiet" aria-label={t.removeAccess(appName(grant.appId))} onClick={() => setRemoving({ id: grant.appId, name: appName(grant.appId) })}>
                <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
            </span>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={AppWindow}
            title={t.noGrants}
            description={t.noGrantsHint}
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
          if (open) return;
          setRemoving(null);
          clearError();
        }}
        error={errorFor("remove")}
        title={t.removeTitle(removing?.name ?? null)}
        description={t.removeHint}
        footer={
          <>
            <Button variant="secondary" onClick={() => setRemoving(null)}>
              {messages.cancel}
            </Button>
            <Button variant="destructive" loading={pending === "remove"} onClick={revoke}>
              {t.removeConfirm}
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
  const { pending, run, errorFor, toast } = useMutation();
  const [appId, setAppId] = useState(grant?.appId ?? "");
  const [scopes, setScopes] = useState<string[]>(grant?.scopes ?? []);
  const [errors, setErrors] = useState<{ app?: string; scopes?: string }>({});
  const messages = useMessages(infrastructureMessages);
  const t = messages.apiResources;

  function save() {
    const next = {
      app: appId ? undefined : t.applicationError,
      scopes: scopes.length ? undefined : t.grantScopesError,
    };
    setErrors(next);
    if (next.app || next.scopes) return;
    void run("save", () => api<ApiResource>(`/api-resources/${resource.id}/grants/${appId}`, { method: "PUT", body: { scopes } }), () => {
      const name = applications.find((a) => a.id === appId)?.name ?? t.theApplication;
      toast({ title: t.granted(name, scopes.join(", ")), description: t.grantedHint });
      onClose();
    }, "inline");
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      error={errorFor("save")}
      title={grant ? t.editAccess(applications[0]?.name ?? null) : t.grantTitle(resource.name)}
      description={t.grantHint}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {messages.cancel}
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {grant ? t.saveAccess : t.grant}
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
        <Field label={t.application} error={errors.app}>
          {(props) => (
            <Select {...props} value={appId} disabled={grant !== null} onChange={(event) => setAppId(event.target.value)}>
              <option value="" disabled>
                {t.chooseApplication}
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
          <legend className="mb-3 text-label text-fg">{t.scopes}</legend>
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
