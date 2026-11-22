"use client";

import Link from "next/link";
import { Plus, X } from "lucide-react";
import { useId, useState, type FormEvent, type ReactNode } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { PeoplePicker } from "@/components/governance/people-picker";
import { ActionError } from "@/components/ui/action-error";
import { Tag } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { Checkbox, Field, Input, Select, Textarea } from "@/components/ui/input";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import type { Locale } from "@/lib/i18n/locale";
import type { ApiResource } from "@/lib/infra-types";
import { SIGNING_ALGORITHMS, type Application, type SigningAlgorithm, type User } from "@/lib/types";
import { clientAddressError, originError, uriError } from "./new-application-flow";

const UNITS = [
  ["seconds", 1],
  ["minutes", 60],
  ["hours", 3600],
  ["days", 86400],
] as const;

type Unit = (typeof UNITS)[number][0];
export type Duration = { amount: string; unit: Unit };
type Details = Pick<Application, "id" | "name" | "description" | "homepage" | "owner"> & { bookmark?: boolean; remote?: boolean };

export function split(seconds: number): Duration {
  const [unit, size] = [...UNITS].reverse().find(([, size]) => seconds % size === 0) ?? UNITS[0];
  return { amount: String(seconds / size), unit };
}

export function toSeconds({ amount, unit }: Duration): number {
  return Number(amount) * (UNITS.find(([u]) => u === unit)?.[1] ?? 1);
}

export const wholeNumber = (d: Duration, locale: Locale = "en") => (/^\d+$/.test(d.amount.trim()) ? undefined : applicationsMessages[locale].edit.wholeNumber);

export function EditDetailsDialog({ app, directory, open, onOpenChange }: { app: Details; directory: User[]; open: boolean; onOpenChange: (open: boolean) => void }) {
  const formId = useId();
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const t = useMessages(applicationsMessages);

  function save(body: { name: string; description: string; homepage: string; owner: string }) {
    void run(
      "details",
      () => api(`/applications/${app.id}`, { method: "PATCH", body }),
      () => {
        onOpenChange(false);
        toast({ title: t.common.savedToast(body.name), description: t.common.audit });
      },
      "inline",
    );
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        clearError();
        onOpenChange(next);
      }}
      error={errorFor("details")}
      title={t.edit.title}
      description={t.edit.description}
      footer={
        <>
          <Button variant="secondary" onClick={() => onOpenChange(false)}>
            {t.common.cancel}
          </Button>
          <Button type="submit" form={formId} variant="primary" loading={pending === "details"}>
            {t.edit.saveDetails}
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
  const t = useMessages(applicationsMessages);

  function submit(event: FormEvent) {
    event.preventDefault();
    const next = {
      name: name.trim() ? undefined : t.setup.validation.nameSignIn,
      homepage:
        !homepage.trim() && app.bookmark
          ? t.setup.validation.bookmarkEmpty
          : !homepage.trim() || /^https?:\/\/[^/\s]+/.test(homepage.trim())
            ? undefined
            : t.edit.fullAddress,
    };
    setErrors(next);
    if (next.name || next.homepage) return;
    onSave({ name: name.trim(), description: description.trim(), homepage: homepage.trim(), owner });
  }

  return (
    <form id={id} className="flex flex-col gap-6" onSubmit={submit} noValidate>
      <Field label={t.common.name} error={errors.name}>
        {(props) => <Input {...props} autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} />}
      </Field>
      <Field label={t.common.description} hint={t.edit.descriptionHint}>
        {(props) => <Textarea {...props} rows={2} value={description} onChange={(event) => setDescription(event.target.value)} />}
      </Field>
      {app.remote ? null : (
        <Field
          label={app.bookmark ? t.common.url : t.edit.homepage}
          error={errors.homepage}
          hint={app.bookmark ? t.edit.bookmarkUrlHint : t.edit.homepageHint}
        >
          {(props) => <Input {...props} type="url" mono value={homepage} onChange={(event) => setHomepage(event.target.value)} placeholder="https://grafana.example.com" />}
        </Field>
      )}
      <Field label={t.common.owner} hint={t.edit.ownerHint}>
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

export function AuthenticationForm({ app, resources, children }: { app: Application; resources: ApiResource[]; children: ReactNode }) {
  const service = app.type === "service";
  const publicClient = app.type === "spa" || app.type === "native";
  const policy = app.tokenPolicy;
  const [redirects, setRedirects] = useState(app.redirectUris);
  const [postLogout, setPostLogout] = useState(app.postLogoutUris);
  const [access, setAccess] = useState(split(policy.accessTokenTtl));
  const [idToken, setIdToken] = useState(split(policy.idTokenTtl));
  const [idTokenAlg, setIdTokenAlg] = useState(policy.idTokenAlg);
  const [refresh, setRefresh] = useState(split(policy.refreshTokenTtl));
  const [rotateRefresh, setRotation] = useState(policy.rotation);
  const rotation = !app.fapi2Profile && rotateRefresh;
  const [exchange, setExchange] = useState(policy.exchange);
  const [exchangeFrom, setExchangeFrom] = useState(policy.exchangeFrom);
  const sourcesId = useId();
  const [errors, setErrors] = useState<{ access?: string; id?: string; refresh?: string; redirects?: string; redirect?: (string | undefined)[]; postLogout?: (string | undefined)[] }>({});
  const { pending, run, errorFor, toast } = useMutation();
  const locale = useLocale();
  const t = applicationsMessages[locale];
  const e = t.edit;
  const refreshOff = refresh.amount.trim() === "0";

  function submit(event: FormEvent) {
    event.preventDefault();
    const clean = (uris: string[]) => uris.map((uri) => uri.trim()).filter(Boolean);
    const check = (uris: string[]) => uris.map((uri) => (uri.trim() ? uriError(uri, locale) : undefined));
    const next = {
      access: wholeNumber(access, locale),
      id: service ? undefined : wholeNumber(idToken, locale),
      refresh: service ? undefined : wholeNumber(refresh, locale),
      redirects: !service && app.protocol === "oidc" && clean(redirects).length === 0 ? e.redirectsRequired : undefined,
      redirect: service ? [] : check(redirects),
      postLogout: service ? [] : check(postLogout),
    };
    setErrors(next);
    if (next.access || next.id || next.refresh || next.redirects || [...next.redirect, ...next.postLogout].some(Boolean)) return;
    const tokenPolicy = service
      ? { accessTokenTtl: toSeconds(access), exchange, exchangeFrom }
      : { accessTokenTtl: toSeconds(access), idTokenTtl: toSeconds(idToken), idTokenAlg, refreshTokenTtl: toSeconds(refresh), rotation, exchange, exchangeFrom };
    void run(
      "authentication",
      () =>
        api(`/applications/${app.id}`, {
          method: "PATCH",
          body: service ? { tokenPolicy } : { tokenPolicy, redirectUris: clean(redirects), postLogoutUris: clean(postLogout) },
        }),
      () => toast({ title: e.authenticationSaved, description: e.nextToken(app.name) }),
      "inline",
    );
  }

  return (
    <form onSubmit={submit} noValidate className="grid grid-cols-1 gap-6 xl:grid-cols-2">
      {children}
      <Card>
        <CardHeader title={e.lifetimes} description={e.lifetimesHint} />
        <CardBody className="grid grid-cols-1 gap-6 sm:grid-cols-2">
          <DurationField label={e.accessToken} hint={e.dayRange} value={access} onChange={setAccess} error={errors.access} />
          {service ? null : (
            <>
              <DurationField label={e.idToken} hint={e.dayRange} value={idToken} onChange={setIdToken} error={errors.id} />
              <DurationField label={e.refreshToken} hint={e.refreshHint} value={refresh} onChange={setRefresh} error={errors.refresh} />
              <Field
                label={e.rotation}
                hint={app.fapi2Profile ? e.rotationFapi : refreshOff ? e.rotationUnused : rotation ? e.rotationOn : e.rotationOff}
              >
                {(props) => (
                  <Select {...props} disabled={refreshOff || app.fapi2Profile} value={rotation ? "on" : "off"} onChange={(event) => setRotation(event.target.value === "on")}>
                    <option value="on">{t.common.on}</option>
                    <option value="off">{t.common.off}</option>
                  </Select>
                )}
              </Field>
              <Field
                label={e.idTokenSigning}
                hint={idTokenAlg === "RS256" ? e.rs256 : e.otherAlg(app.name, idTokenAlg)}
              >
                {(props) => (
                  <Select {...props} value={idTokenAlg} onChange={(event) => setIdTokenAlg(event.target.value as SigningAlgorithm)}>
                    {SIGNING_ALGORITHMS.map(([alg, label]) => (
                      <option key={alg} value={alg}>
                        {label}
                      </option>
                    ))}
                  </Select>
                )}
              </Field>
            </>
          )}
          {publicClient ? null : (
            <Field
              label={t.detail.tokenExchange}
              hint={exchange ? e.exchangeOn : e.exchangeOff}
            >
              {(props) => (
                <Select {...props} value={exchange ? "on" : "off"} onChange={(event) => setExchange(event.target.value === "on")}>
                  <option value="on">{t.common.on}</option>
                  <option value="off">{t.common.off}</option>
                </Select>
              )}
            </Field>
          )}
          {publicClient || !exchange ? null : (
            <fieldset className="flex min-w-0 flex-col gap-3 sm:col-span-2" aria-describedby={`${sourcesId}-hint`}>
              <legend className="mb-3 text-label text-fg">{e.acceptFrom}</legend>
              {resources.map((resource) => (
                <label key={resource.id} className="flex cursor-pointer items-start gap-3">
                  <Checkbox
                    className="mt-0.5"
                    checked={exchangeFrom.includes(resource.id)}
                    onChange={(event) => setExchangeFrom((current) => (event.target.checked ? [...current, resource.id] : current.filter((id) => id !== resource.id)))}
                  />
                  <span className="flex min-w-0 flex-col">
                    <span className="text-body-sm text-fg">{resource.name}</span>
                    <span className="font-mono text-code-sm break-all text-fg-3">{resource.identifier}</span>
                  </span>
                </label>
              ))}
              <p id={`${sourcesId}-hint`} className="text-caption text-fg-3">
                {resources.length === 0 ? (
                  <>
                    {e.noApis}{" "}
                    <Link href="/admin/api-resources" className="text-link underline underline-offset-3 hover:text-ember">
                      {e.addApi}
                    </Link>
                  </>
                ) : exchangeFrom.length === 0 ? (
                  e.acceptNone
                ) : (
                  e.acceptOthers
                )}
              </p>
            </fieldset>
          )}
        </CardBody>
      </Card>
      <Card className="xl:col-span-2">
        <CardHeader title={e.redirects} description={service ? e.redirectsService : e.redirectsHint} />
        {service ? null : (
          <CardBody className="grid grid-cols-1 gap-8 md:grid-cols-2">
            <UriList
              legend={e.redirectUris}
              values={redirects}
              onChange={setRedirects}
              errors={errors.redirect}
              error={errors.redirects}
              placeholder={app.type === "native" ? "http://localhost:8000" : "https://app.example.com/callback"}
              empty={e.noRedirects}
            />
            <UriList
              legend={e.postLogout}
              values={postLogout}
              onChange={setPostLogout}
              errors={errors.postLogout}
              placeholder="https://app.example.com/"
              empty={e.noPostLogout}
            />
          </CardBody>
        )}
      </Card>
      <div className="flex flex-wrap items-center justify-end gap-4 xl:col-span-2">
        <ActionError message={errorFor("authentication")} />
        <Button type="submit" variant="primary" loading={pending === "authentication"}>
          {e.saveChanges}
        </Button>
      </div>
    </form>
  );
}

export function SignInFrequencyForm({ app }: { app: Pick<Application, "id" | "name" | "protocol" | "maxAuthAge"> }) {
  const [age, setAge] = useState(split(app.maxAuthAge));
  const [error, setError] = useState<string>();
  const { pending, run, errorFor, toast } = useMutation();
  const locale = useLocale();
  const e = applicationsMessages[locale].edit;

  function submit(event: FormEvent) {
    event.preventDefault();
    const invalid = wholeNumber(age, locale);
    setError(invalid);
    if (invalid) return;
    void run(
      "frequency",
      () => api(`/applications/${app.id}`, { method: "PATCH", body: { maxAuthAge: toSeconds(age) } }),
      () => toast({ title: e.frequencySaved, description: e.nextSignIn(app.name) }),
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader
        title={e.frequency}
        description={
          app.protocol === "saml"
            ? e.frequencySaml(app.name)
            : app.protocol === "forward-auth"
              ? e.frequencyForward(app.name)
            : app.protocol === "radius"
              ? e.frequencyRadius(app.name)
            : app.protocol === "remote"
              ? e.frequencyRemote(app.name)
            : e.frequencyDefault(app.name)
        }
      />
      <form onSubmit={submit} noValidate>
        <CardBody className="grid grid-cols-1 gap-6 md:grid-cols-2">
          <DurationField label={e.signInAgain} hint={e.signInAgainHint} value={age} onChange={setAge} error={error} />
        </CardBody>
        <div className="flex flex-wrap items-center justify-end gap-4 border-t border-border px-6 py-4">
          <ActionError message={errorFor("frequency")} />
          <Button type="submit" variant="primary" loading={pending === "frequency"}>
            {e.saveFrequency}
          </Button>
        </div>
      </form>
    </Card>
  );
}

export function WindowsSignInForm({ app }: { app: Pick<Application, "id" | "name" | "kerberosSignIn"> }) {
  const [allowed, setAllowed] = useState(app.kerberosSignIn);
  const { pending, run, errorFor, toast } = useMutation();
  const e = useMessages(applicationsMessages).edit;

  function submit(event: FormEvent) {
    event.preventDefault();
    void run(
      "windows",
      () => api(`/applications/${app.id}`, { method: "PATCH", body: { kerberosSignIn: allowed } }),
      () => toast({ title: e.windowsSaved, description: e.nextSignIn(app.name) }),
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader title={e.windows} description={e.windowsHint(app.name)} />
      <form onSubmit={submit} noValidate>
        <CardBody className="grid grid-cols-1 gap-6 md:grid-cols-2">
          <Field label={e.windows} hint={allowed ? e.windowsAllowed : e.windowsBlocked(app.name)}>
            {(props) => (
              <Select {...props} value={allowed ? "allowed" : "blocked"} onChange={(event) => setAllowed(event.target.value === "allowed")}>
                <option value="allowed">{e.allowed}</option>
                <option value="blocked">{e.notAllowed}</option>
              </Select>
            )}
          </Field>
        </CardBody>
        <div className="flex flex-wrap items-center justify-end gap-4 border-t border-border px-6 py-4">
          <ActionError message={errorFor("windows")} />
          <Button type="submit" variant="primary" loading={pending === "windows"}>
            {e.saveWindows}
          </Button>
        </div>
      </form>
    </Card>
  );
}

export function BackchannelLogoutForm({ app }: { app: Pick<Application, "id" | "name" | "backchannelLogoutUri" | "backchannelLogoutSessionRequired"> }) {
  const [uri, setUri] = useState(app.backchannelLogoutUri);
  const [sessionRequired, setSessionRequired] = useState(app.backchannelLogoutSessionRequired);
  const [error, setError] = useState<string>();
  const { pending, run, errorFor, toast } = useMutation();
  const t = useMessages(applicationsMessages);
  const e = t.edit;

  function submit(event: FormEvent) {
    event.preventDefault();
    const next = uri.trim() && !/^https:\/\/\S+$/.test(uri.trim()) ? e.logoutInvalid : undefined;
    setError(next);
    if (next) return;
    void run(
      "logout",
      () => api(`/applications/${app.id}`, { method: "PATCH", body: { backchannelLogoutUri: uri.trim(), backchannelLogoutSessionRequired: sessionRequired } }),
      () =>
        toast({
          title: e.logoutSaved,
          description: uri.trim() ? e.logoutOn(app.name) : e.logoutOff(app.name),
        }),
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader
        title={e.logout}
        description={e.logoutHint(app.name)}
      />
      <form onSubmit={submit} noValidate>
        <CardBody className="grid grid-cols-1 gap-6 md:grid-cols-2">
          <Field label={e.logoutUri} error={error} hint={e.logoutUriHint}>
            {(props) => (
              <Input
                {...props}
                mono
                type="url"
                autoComplete="off"
                spellCheck={false}
                value={uri}
                placeholder="https://app.example.com/backchannel-logout"
                onChange={(event) => setUri(event.target.value)}
              />
            )}
          </Field>
          <Field label={e.sid} hint={e.sidHint}>
            {(props) => (
              <Select {...props} value={sessionRequired ? "required" : "optional"} onChange={(event) => setSessionRequired(event.target.value === "required")}>
                <option value="required">{t.common.required}</option>
                <option value="optional">{t.common.notRequired}</option>
              </Select>
            )}
          </Field>
        </CardBody>
        <div className="flex flex-wrap items-center justify-end gap-4 border-t border-border px-6 py-4">
          <ActionError message={errorFor("logout")} />
          <Button type="submit" variant="primary" loading={pending === "logout"}>
            {e.saveLogout}
          </Button>
        </div>
      </form>
    </Card>
  );
}


export function SenderConstraintForm({ app }: { app: Pick<Application, "id" | "name" | "type" | "requirePar" | "requirePkce" | "requireDpop"> }) {
  const service = app.type === "service";
  const publicClient = app.type === "spa" || app.type === "native";
  const [par, setPar] = useState(app.requirePar);
  const [pkce, setPkce] = useState(app.requirePkce);
  const [dpop, setDpop] = useState(app.requireDpop);
  const { pending, run, errorFor, toast } = useMutation();
  const t = useMessages(applicationsMessages);
  const e = t.edit;

  function submit(event: FormEvent) {
    event.preventDefault();
    void run(
      "constraint",
      () => api(`/applications/${app.id}`, { method: "PATCH", body: service ? { requireDpop: dpop } : { requirePar: par, requirePkce: pkce, requireDpop: dpop } }),
      () => toast({ title: e.protectionSaved, description: e.protectionNext(app.name) }),
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader
        title={e.protection}
        description={service ? e.protectionService(app.name) : e.protectionDefault(app.name)}
      />
      <form onSubmit={submit} noValidate>
        <CardBody className="grid grid-cols-1 gap-6 md:grid-cols-3">
          {service ? null : (
            <Field label={e.par} hint={par ? e.parOn : e.parOff}>
              {(props) => (
                <Select {...props} value={par ? "required" : "optional"} onChange={(event) => setPar(event.target.value === "required")}>
                  <option value="required">{t.common.required}</option>
                  <option value="optional">{t.common.notRequired}</option>
                </Select>
              )}
            </Field>
          )}
          {service ? null : (
            <Field label="PKCE" hint={publicClient ? e.pkcePublic : pkce ? e.pkceOn : e.pkceOff}>
              {(props) => (
                <Select {...props} disabled={publicClient} value={publicClient || pkce ? "required" : "optional"} onChange={(event) => setPkce(event.target.value === "required")}>
                  <option value="required">{t.common.required}</option>
                  <option value="optional">{t.common.notRequired}</option>
                </Select>
              )}
            </Field>
          )}
          <Field label={e.dpop} hint={dpop ? e.dpopOn : e.dpopOff}>
            {(props) => (
              <Select {...props} value={dpop ? "required" : "optional"} onChange={(event) => setDpop(event.target.value === "required")}>
                <option value="required">{t.common.required}</option>
                <option value="optional">{t.common.notRequired}</option>
              </Select>
            )}
          </Field>
        </CardBody>
        <div className="flex flex-wrap items-center justify-end gap-4 border-t border-border px-6 py-4">
          <ActionError message={errorFor("constraint")} />
          <Button type="submit" variant="primary" loading={pending === "constraint"}>
            {e.saveProtection}
          </Button>
        </div>
      </form>
    </Card>
  );
}

export function ProtectedAddressesForm({ app }: { app: Pick<Application, "id" | "name" | "protocol" | "redirectUris"> }) {
  const radius = app.protocol === "radius";
  const [addresses, setAddresses] = useState(app.redirectUris);
  const [errors, setErrors] = useState<(string | undefined)[]>([]);
  const [error, setError] = useState<string>();
  const { pending, run, errorFor, toast } = useMutation();
  const locale = useLocale();
  const e = applicationsMessages[locale].edit;

  function submit(event: FormEvent) {
    event.preventDefault();
    const clean = addresses.map((a) => a.trim()).filter(Boolean);
    const next = addresses.map((a) => (a.trim() ? (radius ? clientAddressError(a, locale) : originError(a, locale)) : undefined));
    const empty = clean.length === 0 ? e.addressRequired(radius) : undefined;
    setErrors(next);
    setError(empty);
    if (empty || next.some(Boolean)) return;
    void run(
      "addresses",
      () => api(`/applications/${app.id}`, { method: "PATCH", body: { redirectUris: clean } }),
      () =>
        toast(
          radius
            ? { title: e.clientAddressesSaved, description: e.clientAddressesNext(app.name) }
            : { title: e.protectedSaved, description: e.protectedNext(app.name) },
        ),
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader
        title={radius ? e.clientAddresses : e.protectedAddresses}
        description={radius ? e.clientAddressesHint(app.name) : e.protectedHint(app.name)}
      />
      <form onSubmit={submit} noValidate>
        <CardBody>
          <UriList
            legend={e.addresses}
            values={addresses}
            onChange={setAddresses}
            errors={errors}
            error={error}
            placeholder={radius ? "10.0.0.0/24" : "https://files.example.com"}
            empty={radius ? e.noClientAddresses : e.noProtectedAddresses}
            add={e.addAddress}
          />
        </CardBody>
        <div className="flex flex-wrap items-center justify-end gap-4 border-t border-border px-6 py-4">
          <ActionError message={errorFor("addresses")} />
          <Button type="submit" variant="primary" loading={pending === "addresses"}>
            {e.saveAddresses}
          </Button>
        </div>
      </form>
    </Card>
  );
}

export function DurationField({ label, hint, value, onChange, error }: { label: string; hint: string; value: Duration; onChange: (value: Duration) => void; error?: string }) {
  const e = useMessages(applicationsMessages).edit;
  return (
    <Field label={label} hint={hint} error={error}>
      {(props) => (
        <div className="flex gap-2">
          <Input {...props} type="number" min={0} inputMode="numeric" className="tnum" value={value.amount} onChange={(event) => onChange({ ...value, amount: event.target.value })} />
          <div className="w-32 shrink-0">
            <Select aria-label={e.unit(label)} value={value.unit} onChange={(event) => onChange({ ...value, unit: event.target.value as Unit })}>
              {UNITS.map(([unit]) => (
                <option key={unit} value={unit}>
                  {e.units[unit]}
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
  add,
}: {
  legend: string;
  values: string[];
  onChange: (values: string[]) => void;
  errors?: (string | undefined)[];
  error?: string;
  placeholder: string;
  empty: string;
  add?: string;
}) {
  const id = useId();
  const e = useMessages(applicationsMessages).edit;
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
            <Button size="icon" variant="quiet" aria-label={e.removeEntry(value, index + 1)} onClick={() => onChange(values.filter((_, i) => i !== index))}>
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
        {add ?? e.addUri}
      </Button>
    </fieldset>
  );
}

export function ScopesForm({ app }: { app: Application }) {
  const [scopes, setScopes] = useState(app.scopes);
  const [consent, setConsent] = useState(app.requiresConsent);
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string>();
  const { pending, run, errorFor, toast } = useMutation();
  const t = useMessages(applicationsMessages);
  const e = t.edit;
  const required = (scope: string) => scope === "openid" && app.protocol === "oidc";

  function add() {
    const scope = draft.trim();
    if (!scope || /[\s"\\]/.test(scope)) {
      setError(e.scopeFormat);
      return;
    }
    if (scopes.includes(scope)) {
      setError(e.scopeDuplicate(scope));
      return;
    }
    setScopes([...scopes, scope]);
    setDraft("");
    setError(undefined);
  }

  function save() {
    void run(
      "scopes",
      () => api(`/applications/${app.id}`, { method: "PATCH", body: { scopes, requiresConsent: consent } }),
      () => toast({ title: e.permissionsSaved, description: e.permissionsNext(app.name) }),
      "inline",
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <SimpleTable
        caption={t.detail.scopes}
        head={[e.scope, e.grantsAccess, t.common.actions]}
        rows={scopes.map((scope) => [
          <Tag key="s">{scope}</Tag>,
          scope === "offline_access"
            ? e.offlineAccess(t.duration(app.tokenPolicy.refreshTokenTtl))
            : (e.scopes[scope] ?? e.customScope),
          required(scope) ? (
            <span key="r" className="text-caption text-fg-3">
              {t.common.required}
            </span>
          ) : (
            <Button key="r" size="sm" variant="quiet" onClick={() => setScopes(scopes.filter((s) => s !== scope))}>
              {t.common.remove}
            </Button>
          ),
        ])}
      />
      {app.type === "service" ? null : (
        <Field
          label={e.consent}
          hint={consent ? e.consentOn : e.consentOff}
          className="w-full sm:max-w-md"
        >
          {(props) => (
            <Select {...props} value={consent ? "on" : "off"} onChange={(event) => setConsent(event.target.value === "on")}>
              <option value="on">{e.askEach}</option>
              <option value="off">{e.dontAsk}</option>
            </Select>
          )}
        </Field>
      )}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
        <Field label={e.addScope} error={error} className="w-full sm:max-w-md">
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
                {t.common.add}
              </Button>
            </div>
          )}
        </Field>
        <div className="flex flex-col items-start gap-2 sm:items-end">
          <ActionError message={errorFor("scopes")} />
          <Button variant="primary" loading={pending === "scopes"} onClick={save}>
            {e.savePermissions}
          </Button>
        </div>
      </div>
    </div>
  );
}
