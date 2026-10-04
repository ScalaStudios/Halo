"use client";

import { Plus, X } from "lucide-react";
import { useId, useState, type FormEvent, type ReactNode } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { PeoplePicker } from "@/components/governance/people-picker";
import { Tag } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input, Select, Textarea } from "@/components/ui/input";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { formatDuration } from "@/lib/format";
import type { Application, User } from "@/lib/types";
import { uriError } from "./new-application-flow";

const SCOPES: Record<string, string> = {
  openid: "Signs the person in and issues an ID token.",
  profile: "Name and preferred username.",
  email: "Email address and whether it is verified.",
  groups: "Names of every group the person belongs to.",
  "billing:read": "Read invoices and usage records.",
  "billing:write": "Create, update and void invoices.",
  "usage:report": "Submit metered usage for billing.",
};

const UNITS = [
  ["seconds", 1, "Seconds"],
  ["minutes", 60, "Minutes"],
  ["hours", 3600, "Hours"],
  ["days", 86400, "Days"],
] as const;

type Unit = (typeof UNITS)[number][0];
type Duration = { amount: string; unit: Unit };
type Details = Pick<Application, "id" | "name" | "description" | "homepage" | "owner">;

function split(seconds: number): Duration {
  const [unit, size] = [...UNITS].reverse().find(([, size]) => seconds % size === 0) ?? UNITS[0];
  return { amount: String(seconds / size), unit };
}

function toSeconds({ amount, unit }: Duration): number {
  return Number(amount) * (UNITS.find(([u]) => u === unit)?.[1] ?? 1);
}

const wholeNumber = (d: Duration) => (/^\d+$/.test(d.amount.trim()) ? undefined : "Enter a whole number.");

export function EditDetailsDialog({ app, directory, open, onOpenChange }: { app: Details; directory: User[]; open: boolean; onOpenChange: (open: boolean) => void }) {
  const formId = useId();
  const { pending, run, toast } = useMutation();

  function save(body: { name: string; description: string; homepage: string; owner: string }) {
    void run(
      "details",
      () => api(`/applications/${app.id}`, { method: "PATCH", body }),
      () => {
        onOpenChange(false);
        toast({ title: `${body.name} saved`, description: "Recorded in the audit log." });
      },
    );
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title="Edit details"
      description="People see the name on the Halo sign-in screen and in their app launcher."
      footer={
        <>
          <Button variant="secondary" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button type="submit" form={formId} variant="primary" loading={pending === "details"}>
            Save details
          </Button>
        </>
      }
    >
      <DetailsForm id={formId} app={app} directory={directory} onSave={save} />
    </Dialog>
  );
}

function DetailsForm({
  id,
  app,
  directory,
  onSave,
}: {
  id: string;
  app: Details;
  directory: User[];
  onSave: (body: { name: string; description: string; homepage: string; owner: string }) => void;
}) {
  const [name, setName] = useState(app.name);
  const [description, setDescription] = useState(app.description);
  const [homepage, setHomepage] = useState(app.homepage);
  const [owner, setOwner] = useState(app.owner ?? "");
  const [errors, setErrors] = useState<{ name?: string; homepage?: string }>({});

  function submit(event: FormEvent) {
    event.preventDefault();
    const next = {
      name: name.trim() ? undefined : "Enter a name. People see it on the Halo sign-in screen.",
      homepage: !homepage.trim() || /^https?:\/\/[^/\s]+/.test(homepage.trim()) ? undefined : "Enter a full address, like https://grafana.example.com.",
    };
    setErrors(next);
    if (next.name || next.homepage) return;
    onSave({ name: name.trim(), description: description.trim(), homepage: homepage.trim(), owner });
  }

  return (
    <form id={id} className="flex flex-col gap-6" onSubmit={submit} noValidate>
      <Field label="Name" error={errors.name}>
        {(props) => <Input {...props} autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} />}
      </Field>
      <Field label="Description" hint="Optional. Shown next to the name in the console.">
        {(props) => <Textarea {...props} rows={2} value={description} onChange={(event) => setDescription(event.target.value)} />}
      </Field>
      <Field label="Homepage" error={errors.homepage} hint="Optional. Where the app launcher opens the application.">
        {(props) => <Input {...props} type="url" mono value={homepage} onChange={(event) => setHomepage(event.target.value)} placeholder="https://grafana.example.com" />}
      </Field>
      <Field label="Owner" hint="The person responsible for this application. Optional.">
        {(props) => (
          <PeoplePicker
            {...props}
            people={directory.filter((u) => u.status !== "deprovisioned")}
            value={owner ? [owner] : []}
            onChange={(ids) => setOwner(ids.at(-1) ?? "")}
          />
        )}
      </Field>
    </form>
  );
}

export function AuthenticationForm({ app, children }: { app: Application; children: ReactNode }) {
  const service = app.type === "service";
  const policy = app.tokenPolicy;
  const [redirects, setRedirects] = useState(app.redirectUris);
  const [postLogout, setPostLogout] = useState(app.postLogoutUris);
  const [access, setAccess] = useState(split(policy.accessTokenTtl));
  const [idToken, setIdToken] = useState(split(policy.idTokenTtl));
  const [refresh, setRefresh] = useState(split(policy.refreshTokenTtl));
  const [rotation, setRotation] = useState(policy.rotation);
  const [errors, setErrors] = useState<{ access?: string; id?: string; refresh?: string; redirects?: string; redirect?: (string | undefined)[]; postLogout?: (string | undefined)[] }>({});
  const { pending, run, toast } = useMutation();
  const refreshOff = refresh.amount.trim() === "0";

  function submit(event: FormEvent) {
    event.preventDefault();
    const clean = (uris: string[]) => uris.map((uri) => uri.trim()).filter(Boolean);
    const check = (uris: string[]) => uris.map((uri) => (uri.trim() ? uriError(uri) : undefined));
    const next = {
      access: wholeNumber(access),
      id: service ? undefined : wholeNumber(idToken),
      refresh: service ? undefined : wholeNumber(refresh),
      redirects:
        !service && app.protocol === "oidc" && clean(redirects).length === 0
          ? "Add at least one redirect URI. Halo only sends people back to addresses registered here."
          : undefined,
      redirect: service ? [] : check(redirects),
      postLogout: service ? [] : check(postLogout),
    };
    setErrors(next);
    if (next.access || next.id || next.refresh || next.redirects || [...next.redirect, ...next.postLogout].some(Boolean)) return;
    const tokenPolicy = service
      ? { accessTokenTtl: toSeconds(access) }
      : { accessTokenTtl: toSeconds(access), idTokenTtl: toSeconds(idToken), refreshTokenTtl: toSeconds(refresh), rotation };
    void run(
      "authentication",
      () =>
        api(`/applications/${app.id}`, {
          method: "PATCH",
          body: service ? { tokenPolicy } : { tokenPolicy, redirectUris: clean(redirects), postLogoutUris: clean(postLogout) },
        }),
      () => toast({ title: "Authentication settings saved", description: `${app.name} gets them on its next token request. Recorded in the audit log.` }),
    );
  }

  return (
    <form onSubmit={submit} noValidate className="grid grid-cols-1 gap-6 xl:grid-cols-2">
      {children}
      <Card>
        <CardHeader title="Token lifetimes" description="Shorter access tokens make revocation take effect sooner." />
        <CardBody className="grid grid-cols-1 gap-6 sm:grid-cols-2">
          <DurationField label="Access token" hint="1 minute to 24 hours." value={access} onChange={setAccess} error={errors.access} />
          {service ? null : (
            <>
              <DurationField label="ID token" hint="1 minute to 24 hours." value={idToken} onChange={setIdToken} error={errors.id} />
              <DurationField label="Refresh token" hint="Up to 90 days. Set 0 to stop issuing them." value={refresh} onChange={setRefresh} error={errors.refresh} />
              <Field
                label="Refresh token rotation"
                hint={refreshOff ? "Not used while refresh tokens are off." : rotation ? "Each refresh revokes the previous token." : "The same refresh token works until it expires."}
              >
                {(props) => (
                  <Select {...props} disabled={refreshOff} value={rotation ? "on" : "off"} onChange={(event) => setRotation(event.target.value === "on")}>
                    <option value="on">On</option>
                    <option value="off">Off</option>
                  </Select>
                )}
              </Field>
            </>
          )}
        </CardBody>
      </Card>
      <Card className="xl:col-span-2">
        <CardHeader title="Redirects" description={service ? "Service applications don't use a browser flow." : "Halo only redirects to these exact URIs."} />
        {service ? null : (
          <CardBody className="grid grid-cols-1 gap-8 md:grid-cols-2">
            <UriList
              legend="Redirect URIs"
              values={redirects}
              onChange={setRedirects}
              errors={errors.redirect}
              error={errors.redirects}
              placeholder={app.type === "native" ? "http://localhost:8000" : "https://app.example.com/callback"}
              empty="No redirect URIs."
            />
            <UriList
              legend="Post-logout redirect URIs"
              values={postLogout}
              onChange={setPostLogout}
              errors={errors.postLogout}
              placeholder="https://app.example.com/"
              empty="None. Halo shows its own signed-out page."
            />
          </CardBody>
        )}
      </Card>
      <div className="flex justify-end xl:col-span-2">
        <Button type="submit" variant="primary" loading={pending === "authentication"}>
          Save changes
        </Button>
      </div>
    </form>
  );
}

function DurationField({ label, hint, value, onChange, error }: { label: string; hint: string; value: Duration; onChange: (value: Duration) => void; error?: string }) {
  return (
    <Field label={label} hint={hint} error={error}>
      {(props) => (
        <div className="flex gap-2">
          <Input {...props} type="number" min={0} inputMode="numeric" className="tnum" value={value.amount} onChange={(event) => onChange({ ...value, amount: event.target.value })} />
          <div className="w-32 shrink-0">
            <Select aria-label={`${label} unit`} value={value.unit} onChange={(event) => onChange({ ...value, unit: event.target.value as Unit })}>
              {UNITS.map(([unit, , text]) => (
                <option key={unit} value={unit}>
                  {text}
                </option>
              ))}
            </Select>
          </div>
        </div>
      )}
    </Field>
  );
}

function UriList({
  legend,
  values,
  onChange,
  errors = [],
  error,
  placeholder,
  empty,
}: {
  legend: string;
  values: string[];
  onChange: (values: string[]) => void;
  errors?: (string | undefined)[];
  error?: string;
  placeholder: string;
  empty: string;
}) {
  const id = useId();
  return (
    <fieldset className="flex min-w-0 flex-col gap-2" aria-describedby={error ? `${id}-error` : undefined}>
      <legend className="mb-2 text-label text-fg">{legend}</legend>
      {values.length === 0 ? <p className="text-body-sm text-fg-3">{empty}</p> : null}
      {values.map((value, index) => (
        <div key={index} className="flex flex-col gap-1">
          <div className="flex items-center gap-2">
            <Input
              mono
              aria-label={`${legend} ${index + 1}`}
              aria-invalid={errors[index] ? true : undefined}
              aria-describedby={errors[index] ? `${id}-${index}` : undefined}
              value={value}
              placeholder={placeholder}
              onChange={(event) => onChange(values.map((v, i) => (i === index ? event.target.value : v)))}
            />
            <Button size="icon" variant="quiet" aria-label={`Remove ${value || `entry ${index + 1}`}`} onClick={() => onChange(values.filter((_, i) => i !== index))}>
              <X aria-hidden="true" size={16} strokeWidth={1.75} />
            </Button>
          </div>
          {errors[index] ? (
            <p id={`${id}-${index}`} className="text-caption text-danger">
              {errors[index]}
            </p>
          ) : null}
        </div>
      ))}
      {error ? (
        <p id={`${id}-error`} className="text-caption text-danger">
          {error}
        </p>
      ) : null}
      <Button size="sm" variant="quiet" className="self-start" onClick={() => onChange([...values, ""])}>
        <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
        Add URI
      </Button>
    </fieldset>
  );
}

export function ScopesForm({ app }: { app: Application }) {
  const [scopes, setScopes] = useState(app.scopes);
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string>();
  const { pending, run, toast } = useMutation();
  const required = (scope: string) => scope === "openid" && app.protocol === "oidc";

  function add() {
    const scope = draft.trim();
    if (!scope || /[\s"\\]/.test(scope)) {
      setError("Scopes are single words without spaces or quotes, such as billing:read.");
      return;
    }
    if (scopes.includes(scope)) {
      setError(`${scope} is already in the list.`);
      return;
    }
    setScopes([...scopes, scope]);
    setDraft("");
    setError(undefined);
  }

  function save() {
    void run(
      "scopes",
      () => api(`/applications/${app.id}`, { method: "PATCH", body: { scopes } }),
      () => toast({ title: "Scopes saved", description: `${app.name} can request them from its next sign-in. Recorded in the audit log.` }),
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <SimpleTable
        caption="Scopes"
        head={["Scope", "Grants access to", "Actions"]}
        rows={scopes.map((scope) => [
          <Tag key="s">{scope}</Tag>,
          scope === "offline_access"
            ? `Refresh tokens valid for ${formatDuration(app.tokenPolicy.refreshTokenTtl)}, so people stay signed in without opening a browser again.`
            : (SCOPES[scope] ?? "Custom scope defined for this application."),
          required(scope) ? (
            <span key="r" className="text-caption text-fg-3">
              Required
            </span>
          ) : (
            <Button key="r" size="sm" variant="quiet" onClick={() => setScopes(scopes.filter((s) => s !== scope))}>
              Remove
            </Button>
          ),
        ])}
      />
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <Field label="Add a scope" error={error} className="w-full sm:max-w-md">
          {(props) => (
            <div className="flex gap-2">
              <Input
                {...props}
                mono
                autoComplete="off"
                spellCheck={false}
                value={draft}
                placeholder="offline_access"
                onChange={(event) => setDraft(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    add();
                  }
                }}
              />
              <Button variant="secondary" onClick={add}>
                Add
              </Button>
            </div>
          )}
        </Field>
        <Button variant="primary" loading={pending === "scopes"} onClick={save} className="self-start sm:self-auto">
          Save scopes
        </Button>
      </div>
    </div>
  );
}
