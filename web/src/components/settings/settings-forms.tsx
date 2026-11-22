"use client";

import { Plus, Trash2, TriangleAlert } from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { ActionError } from "@/components/ui/action-error";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { CheckList, FieldError } from "@/components/policy/policy-form";
import { joinList } from "@/components/policy/shared";
import { Checkbox, Field, Input, Select, Textarea } from "@/components/ui/input";
import { useLocale, useMessages } from "@/lib/i18n/client";
import { settingsMessages } from "@/lib/i18n/console/settings";
import { languageNames, locales, type Locale } from "@/lib/i18n/locale";
import type { Domain, EmailTemplate, Settings } from "@/lib/settings-types";
import type { Group } from "@/lib/types";
import { ReadOnlyNote, useSaveSettings } from "./shared";

export function OrganizationForm({ settings, defaultName, issuer, canEdit }: { settings: Settings; defaultName: string; issuer: string; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const [name, setName] = useState(settings.organizationName);
  const [email, setEmail] = useState(settings.contactEmail);
  const [locale, setLocale] = useState(settings.locale);
  const [error, setError] = useState<string>();
  const t = useMessages(settingsMessages).organization;
  const dirty = name.trim() !== settings.organizationName || email.trim() !== settings.contactEmail || locale !== settings.locale;

  function submit() {
    if (email.trim() && !/^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/.test(email.trim())) {
      setError(t.invalidEmail);
      return;
    }
    setError(undefined);
    void save({ organizationName: name.trim(), contactEmail: email.trim(), locale }, t.saved, t.savedHint);
  }

  return (
    <Card>
      <CardHeader title={t.title} description={t.titleHint} />
      <CardBody>
        <form
          noValidate
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label={t.name} hint={t.nameHint(defaultName)}>
            {(props) => <Input {...props} maxLength={100} autoComplete="organization" disabled={!canEdit} value={name} placeholder={defaultName} onChange={(event) => setName(event.target.value)} />}
          </Field>
          <Field label={t.email} error={error} hint={t.emailHint}>
            {(props) => <Input {...props} type="email" autoComplete="email" disabled={!canEdit} value={email} placeholder="it@example.com" onChange={(event) => setEmail(event.target.value)} />}
          </Field>
          <Field label={t.language} hint={t.languageHint}>
            {(props) => (
              <Select {...props} disabled={!canEdit} value={locale} onChange={(event) => setLocale(event.target.value as Locale)}>
                {locales.map((value) => (
                  <option key={value} value={value} lang={value}>
                    {languageNames[value]}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <CopyField label={t.issuer} value={issuer} hint={t.issuerHint} />
          {canEdit ? (
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                {t.save}
              </Button>
              <ActionError message={failure} />
            </div>
          ) : (
            <ReadOnlyNote>{t.readOnly}</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}

export function SignInMessageForm({ settings, canEdit }: { settings: Settings; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const [message, setMessage] = useState(settings.signInMessage);
  const t = useMessages(settingsMessages).signInMessage;

  return (
    <Card>
      <CardHeader title={t.title} description={t.description} />
      <CardBody>
        <form
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            void save({ signInMessage: message.trim() }, message.trim() ? t.saved : t.removed, t.savedHint);
          }}
        >
          <Field label={t.label} hint={t.hint(message.length)}>
            {(props) => (
              <Textarea {...props} rows={4} maxLength={500} disabled={!canEdit} value={message} placeholder={t.placeholder} onChange={(event) => setMessage(event.target.value)} />
            )}
          </Field>
          {canEdit ? (
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="secondary" loading={saving} disabled={message.trim() === settings.signInMessage}>
                {t.save}
              </Button>
              <ActionError message={failure} />
            </div>
          ) : (
            <ReadOnlyNote>{t.readOnly}</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}

const CSS_LIMIT = 32 * 1024;

export function CustomCssForm({ settings, disabled, canEdit }: { settings: Settings; disabled: boolean; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const [css, setCss] = useState(settings.customCss);
  const [inConsole, setInConsole] = useState(settings.consoleBranding);
  const locale = useLocale();
  const t = settingsMessages[locale].customCss;
  const bytes = new TextEncoder().encode(css.trim()).length;
  const error =
    bytes > CSS_LIMIT
      ? t.tooLarge((bytes - CSS_LIMIT).toLocaleString(locale))
      : css.includes("</")
        ? t.closingTag
        : undefined;

  return (
    <Card>
      <CardHeader
        title={t.title}
        description={t.description}
      />
      <CardBody>
        <form
          className="flex flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            void save({ customCss: css.trim(), consoleBranding: inConsole }, t.saved, t.savedHint);
          }}
        >
          {disabled ? (
            <div role="note" className="flex gap-3 rounded-md border border-warning/20 bg-warning-container p-4">
              <TriangleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0 text-warning" />
              <p className="text-body-sm text-fg-2">
                {t.disabledBefore}
                <code className="text-code-sm">HALO_DISABLE_CUSTOM_CSS=1</code>
                {t.disabledAfter}
              </p>
            </div>
          ) : null}
          <Field label="CSS" error={error} hint={t.hint(bytes.toLocaleString(locale))}>
            {(props) => (
              <Textarea
                {...props}
                mono
                rows={10}
                spellCheck={false}
                autoComplete="off"
                disabled={!canEdit}
                value={css}
                placeholder={":root {\n  --radius-md: 2px;\n  --font-display: Georgia, serif;\n}"}
                onChange={(event) => setCss(event.target.value)}
              />
            )}
          </Field>
          <label className="flex cursor-pointer items-start gap-3">
            <Checkbox className="mt-0.5" disabled={!canEdit} checked={inConsole} onChange={(event) => setInConsole(event.target.checked)} />
            <span className="flex flex-col">
              <span className="text-body-sm text-fg">{t.inConsole}</span>
              <span className="text-caption text-fg-3">{t.inConsoleHint}</span>
            </span>
          </label>
          {canEdit ? (
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="secondary" loading={saving} disabled={Boolean(error) || (css.trim() === settings.customCss && inConsole === settings.consoleBranding)}>
                {t.save}
              </Button>
              <ActionError message={failure} />
            </div>
          ) : (
            <ReadOnlyNote>{t.readOnly}</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}

type NumberKey = "sessionHours" | "reauthMinutes" | "lockoutThreshold" | "lockoutMinutes" | "inviteDays" | "accessTokenMinutes" | "idTokenMinutes" | "refreshTokenHours";

type NumberSpec = { key: NumberKey; unit: "hours" | "minutes" | "attempts" | "days"; min: number; max: number; global?: boolean };

const GROUPS: { key: "sessions" | "lockout" | "setup" | "applications"; fields: NumberSpec[] }[] = [
  {
    key: "sessions",
    fields: [
      { key: "sessionHours", unit: "hours", min: 1, max: 72 },
      { key: "reauthMinutes", unit: "minutes", min: 1, max: 1440, global: true },
    ],
  },
  {
    key: "lockout",
    fields: [
      { key: "lockoutThreshold", unit: "attempts", min: 3, max: 20 },
      { key: "lockoutMinutes", unit: "minutes", min: 5, max: 1440 },
    ],
  },
  { key: "setup", fields: [{ key: "inviteDays", unit: "days", min: 1, max: 30 }] },
  {
    key: "applications",
    fields: [
      { key: "accessTokenMinutes", unit: "minutes", min: 5, max: 1440 },
      { key: "idTokenMinutes", unit: "minutes", min: 5, max: 1440 },
      { key: "refreshTokenHours", unit: "hours", min: 1, max: 2160 },
    ],
  },
];

const FIELDS = GROUPS.flatMap((group) => group.fields);

export function SecurityForm({ settings, canEdit, canEditGlobal }: { settings: Settings; canEdit: boolean; canEditGlobal: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const [values, setValues] = useState(() => Object.fromEntries(FIELDS.map((f) => [f.key, String(settings[f.key])])) as Record<NumberKey, string>);
  const [errors, setErrors] = useState<Partial<Record<NumberKey, string>>>({});
  const locale = useLocale();
  const t = settingsMessages[locale].security;
  const dirty = FIELDS.some((f) => Number(values[f.key]) !== settings[f.key]);

  function submit() {
    const next: Partial<Record<NumberKey, string>> = {};
    for (const f of FIELDS) {
      const value = Number(values[f.key]);
      if (!Number.isInteger(value) || value < f.min || value > f.max) next[f.key] = t.invalid(f.min.toLocaleString(locale), f.max.toLocaleString(locale));
    }
    setErrors(next);
    if (Object.keys(next).length) return;
    const body = Object.fromEntries(FIELDS.map((f) => [f.key, Number(values[f.key])]));
    void save(body, t.saved, t.savedHint);
  }

  return (
    <form
      noValidate
      className="flex flex-col gap-6"
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
    >
      {GROUPS.map((group) => (
        <Card key={group.key}>
          <CardHeader title={t.groups[group.key].title} description={t.groups[group.key].description} />
          <CardBody className="grid grid-cols-1 gap-6 md:grid-cols-3">
            {group.fields.map((f) => (
              <Field key={f.key} label={t.fields[f.key].label} hint={t.fields[f.key].hint} error={errors[f.key]}>
                {(props) => (
                  <div className="flex items-center gap-3">
                    <div className="w-32">
                      <Input
                        {...props}
                        type="number"
                        inputMode="numeric"
                        min={f.min}
                        max={f.max}
                        step={1}
                        disabled={!canEdit || (f.global && !canEditGlobal)}
                        className="tnum"
                        value={values[f.key]}
                        onChange={(event) => setValues((current) => ({ ...current, [f.key]: event.target.value }))}
                      />
                    </div>
                    <span className="text-body-sm text-fg-3">{t.units[f.unit]}</span>
                  </div>
                )}
              </Field>
            ))}
          </CardBody>
        </Card>
      ))}
      {canEdit ? (
        <div className="flex flex-wrap items-center gap-4">
          <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
            {t.save}
          </Button>
          <ActionError message={failure} />
        </div>
      ) : (
        <ReadOnlyNote>{t.readOnly}</ReadOnlyNote>
      )}
    </form>
  );
}

const REGISTRATION: Settings["clientRegistration"][] = ["off", "token", "approval", "open"];

export function ClientRegistrationForm({ settings, issuer, canEdit }: { settings: Settings; issuer: string; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const [mode, setMode] = useState(settings.clientRegistration);
  const { clientRegistration: t, common } = useMessages(settingsMessages);

  return (
    <Card>
      <CardHeader
        title={t.title}
        description={t.description}
      />
      <CardBody>
        <form
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            void save({ clientRegistration: mode }, mode === "off" ? t.turnedOff : t.saved, common.everyServer);
          }}
        >
          <Field label={t.label} hint={t.modes[mode].hint}>
            {(props) => (
              <Select {...props} disabled={!canEdit} value={mode} onChange={(event) => setMode(event.target.value as Settings["clientRegistration"])}>
                {REGISTRATION.map((value) => (
                  <option key={value} value={value}>
                    {t.modes[value].label}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          {mode === "off" ? null : <CopyField label={t.endpoint} value={`${issuer}/oauth2/register`} hint={t.endpointHint} />}
          {canEdit ? (
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={saving} disabled={mode === settings.clientRegistration}>
                {t.save}
              </Button>
              <ActionError message={failure} />
            </div>
          ) : (
            <ReadOnlyNote>{t.readOnly}</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}

function sessionCount(value: string, min: number) {
  const n = Number(value);
  return value.trim() !== "" && Number.isInteger(n) && n >= min && n <= 100;
}

export function SessionLimitForm({ settings, groups, canEdit }: { settings: Settings; groups: Group[]; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const [limit, setLimit] = useState(String(settings.sessionLimit));
  const [action, setAction] = useState(settings.sessionLimitAction);
  const [cli, setCli] = useState(settings.sessionLimitCli);
  const [rows, setRows] = useState(() =>
    settings.sessionLimitGroups.filter((row) => groups.some((g) => g.id === row.groupId)).map((row) => ({ groupId: row.groupId, limit: String(row.limit) })),
  );
  const [attempted, setAttempted] = useState(false);
  const { sessionLimit: t, common } = useMessages(settingsMessages);
  const limitError = sessionCount(limit, 0) ? undefined : t.limitError;
  const rowsError = rows.some((row) => !row.groupId)
    ? t.rowGroupError
    : rows.some((row) => !sessionCount(row.limit, 1))
      ? t.rowLimitError
      : undefined;
  const body = { sessionLimit: Number(limit), sessionLimitAction: action, sessionLimitCli: cli, sessionLimitGroups: rows.map((row) => ({ groupId: row.groupId, limit: Number(row.limit) })) };
  const saved = { sessionLimit: settings.sessionLimit, sessionLimitAction: settings.sessionLimitAction, sessionLimitCli: settings.sessionLimitCli, sessionLimitGroups: settings.sessionLimitGroups };
  const dirty = JSON.stringify(body) !== JSON.stringify(saved);
  const groupName = (id: string) => groups.find((g) => g.id === id)?.name ?? t.thisGroup;

  function update(index: number, change: Partial<(typeof rows)[number]>) {
    setRows((current) => current.map((row, i) => (i === index ? { ...row, ...change } : row)));
  }

  function submit() {
    setAttempted(true);
    if (limitError || rowsError) return;
    void save(body, t.saved, t.savedHint);
  }

  return (
    <Card>
      <CardHeader title={t.title} description={t.description} />
      <CardBody>
        <form
          noValidate
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label={t.perPerson} hint={t.perPersonHint} error={attempted ? limitError : undefined}>
            {(props) => (
              <div className="flex items-center gap-3">
                <div className="w-32">
                  <Input {...props} type="number" inputMode="numeric" min={0} max={100} step={1} disabled={!canEdit} className="tnum" value={limit} onChange={(event) => setLimit(event.target.value)} />
                </div>
                <span className="text-body-sm text-fg-3">{t.sessions}</span>
              </div>
            )}
          </Field>
          <Field label={t.action} hint={t.actionHint}>
            {(props) => (
              <Select {...props} disabled={!canEdit} value={action} onChange={(event) => setAction(event.target.value as Settings["sessionLimitAction"])}>
                <option value="refuse">{t.refuse}</option>
                <option value="end-oldest">{t.endOldest}</option>
              </Select>
            )}
          </Field>
          <label className="flex cursor-pointer items-start gap-3">
            <Checkbox className="mt-0.5" disabled={!canEdit} checked={cli} onChange={(event) => setCli(event.target.checked)} />
            <span className="flex flex-col">
              <span className="text-body-sm text-fg">{t.cli}</span>
              <span className="text-caption text-fg-3">
                {t.cliBefore}
                <code className="text-code-sm">halo login</code>
                {t.cliAfter}
              </span>
            </span>
          </label>
          <fieldset className="flex flex-col gap-3">
            <legend className="mb-1 text-label text-fg">{t.groupLimits}</legend>
            <p className="text-caption text-fg-3">{t.groupLimitsHint}</p>
            {rows.map((row, index) => (
              <div key={index} className="flex items-center gap-3">
                <Select aria-label={t.group(index + 1)} disabled={!canEdit} value={row.groupId} onChange={(event) => update(index, { groupId: event.target.value })}>
                  <option value="">{common.chooseGroup}</option>
                  {groups
                    .filter((g) => g.id === row.groupId || !rows.some((other) => other.groupId === g.id))
                    .map((g) => (
                      <option key={g.id} value={g.id}>
                        {g.name}
                      </option>
                    ))}
                </Select>
                <div className="w-24 shrink-0">
                  <Input
                    aria-label={t.sessionsFor(groupName(row.groupId))}
                    type="number"
                    inputMode="numeric"
                    min={1}
                    max={100}
                    step={1}
                    disabled={!canEdit}
                    className="tnum"
                    value={row.limit}
                    onChange={(event) => update(index, { limit: event.target.value })}
                  />
                </div>
                {canEdit ? (
                  <Button size="icon-sm" variant="quiet" aria-label={t.removeLimit(groupName(row.groupId))} onClick={() => setRows((current) => current.filter((_, i) => i !== index))}>
                    <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                  </Button>
                ) : null}
              </div>
            ))}
            {canEdit ? (
              <div>
                <Button size="sm" variant="secondary" disabled={rows.length >= groups.length} onClick={() => setRows((current) => [...current, { groupId: "", limit: "1" }])}>
                  <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
                  {t.addGroupLimit}
                </Button>
              </div>
            ) : null}
            <FieldError>{attempted ? rowsError : undefined}</FieldError>
          </fieldset>
          {canEdit ? (
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                {t.save}
              </Button>
              <ActionError message={failure} />
            </div>
          ) : (
            <ReadOnlyNote>{t.readOnly}</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}

export function BotProtectionForm({ settings, secretSet, canEdit }: { settings: Settings; secretSet: boolean; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const [provider, setProvider] = useState(settings.botProtection);
  const [siteKey, setSiteKey] = useState(settings.botProtectionSiteKey);
  const [secret, setSecret] = useState("");
  const [attempted, setAttempted] = useState(false);
  const { botProtection: t, common } = useMessages(settingsMessages);
  const siteKeyError = provider && !/^[A-Za-z0-9_-]{1,100}$/.test(siteKey.trim()) ? t.siteKeyError : undefined;
  const secretError = provider && !secretSet && !secret.trim() ? t.secretError : undefined;
  const dirty = provider !== settings.botProtection || siteKey.trim() !== settings.botProtectionSiteKey || secret.trim() !== "";

  function submit() {
    setAttempted(true);
    if (siteKeyError || secretError) return;
    void save(
      { botProtection: provider, botProtectionSiteKey: siteKey.trim(), ...(secret.trim() ? { botProtectionSecret: secret.trim() } : {}) },
      provider ? t.saved : t.turnedOff,
      t.savedHint,
    ).then((saved) => saved && setSecret(""));
  }

  return (
    <Card>
      <CardHeader
        title={t.title}
        description={t.description}
      />
      <CardBody>
        <form
          noValidate
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label={common.provider} hint={t.providerHint}>
            {(props) => (
              <Select {...props} disabled={!canEdit} value={provider} onChange={(event) => setProvider(event.target.value as Settings["botProtection"])}>
                <option value="">{common.off}</option>
                <option value="turnstile">Cloudflare Turnstile</option>
                <option value="hcaptcha">hCaptcha</option>
              </Select>
            )}
          </Field>
          {provider ? (
            <>
              <Field label={t.siteKey} hint={t.siteKeyHint} error={attempted ? siteKeyError : undefined}>
                {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} disabled={!canEdit} value={siteKey} onChange={(event) => setSiteKey(event.target.value)} />}
              </Field>
              <Field
                label={t.secretKey}
                hint={secretSet ? common.keepSecret : common.storedEncrypted}
                error={attempted ? secretError : undefined}
              >
                {(props) => (
                  <Input {...props} mono type="password" autoComplete="new-password" spellCheck={false} disabled={!canEdit} value={secret} onChange={(event) => setSecret(event.target.value)} />
                )}
              </Field>
            </>
          ) : null}
          {canEdit ? (
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                {t.save}
              </Button>
              <ActionError message={failure} />
            </div>
          ) : (
            <ReadOnlyNote>{t.readOnly}</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}

type RetentionKey = "signInLogDays" | "auditLogDays";

const RETENTION: { key: RetentionKey; min: number }[] = [
  { key: "signInLogDays", min: 7 },
  { key: "auditLogDays", min: 30 },
];

export function LogRetentionForm({ settings, canEdit }: { settings: Settings; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const [values, setValues] = useState({ signInLogDays: String(settings.signInLogDays), auditLogDays: String(settings.auditLogDays) });
  const [attempted, setAttempted] = useState(false);
  const t = useMessages(settingsMessages).retention;
  const days = (key: RetentionKey) => Number(values[key]);
  const invalid = (f: (typeof RETENTION)[number]) => values[f.key].trim() === "" || !Number.isInteger(days(f.key)) || (days(f.key) !== 0 && (days(f.key) < f.min || days(f.key) > 3650));
  const dirty = RETENTION.some((f) => days(f.key) !== settings[f.key]);
  const shortened = RETENTION.some((f) => !invalid(f) && days(f.key) > 0 && (settings[f.key] === 0 || days(f.key) < settings[f.key]));

  function submit() {
    setAttempted(true);
    if (RETENTION.some(invalid)) return;
    void save({ signInLogDays: days("signInLogDays"), auditLogDays: days("auditLogDays") }, t.saved, t.savedHint);
  }

  return (
    <Card>
      <CardHeader title={t.title} description={t.description} />
      <CardBody>
        <form
          noValidate
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          {RETENTION.map((f) => (
            <Field key={f.key} label={t[f.key].label} hint={t[f.key].hint} error={attempted && invalid(f) ? t.invalid(f.min) : undefined}>
              {(props) => (
                <div className="flex items-center gap-3">
                  <div className="w-32">
                    <Input
                      {...props}
                      type="number"
                      inputMode="numeric"
                      min={0}
                      max={3650}
                      step={1}
                      disabled={!canEdit}
                      className="tnum"
                      value={values[f.key]}
                      onChange={(event) => setValues((current) => ({ ...current, [f.key]: event.target.value }))}
                    />
                  </div>
                  <span className="text-body-sm text-fg-3">{t.days}</span>
                </div>
              )}
            </Field>
          ))}
          {shortened ? (
            <div role="note" className="flex gap-3 rounded-md border border-warning/20 bg-warning-container p-4">
              <TriangleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0 text-warning" />
              <p className="text-body-sm text-fg-2">{t.shortened}</p>
            </div>
          ) : null}
          {canEdit ? (
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                {t.save}
              </Button>
              <ActionError message={failure} />
            </div>
          ) : (
            <ReadOnlyNote>{t.readOnly}</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}

const SCHEDULES: Settings["exportSchedule"][] = ["off", "daily", "weekly"];

export function ScheduledExportForm({ settings, bucketConfigured, canEdit }: { settings: Settings; bucketConfigured: boolean; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const [schedule, setSchedule] = useState(settings.exportSchedule);
  const [keep, setKeep] = useState(String(settings.exportKeep));
  const [attempted, setAttempted] = useState(false);
  const t = useMessages(settingsMessages).scheduledExports;
  const count = Number(keep);
  const invalid = keep.trim() === "" || !Number.isInteger(count) || count < 1 || count > 365;
  const dirty = schedule !== settings.exportSchedule || count !== settings.exportKeep;
  const editable = canEdit && (bucketConfigured || settings.exportSchedule !== "off");

  function submit() {
    setAttempted(true);
    if (invalid) return;
    void save({ exportSchedule: schedule, exportKeep: count }, schedule === "off" ? t.turnedOff : t.saved, t.savedHint(count));
  }

  return (
    <Card>
      <CardHeader title={t.title} description={t.description} />
      <CardBody>
        <form
          noValidate
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label={t.schedule} hint={t.schedules[schedule].hint}>
            {(props) => (
              <Select {...props} disabled={!editable} value={schedule} onChange={(event) => setSchedule(event.target.value as Settings["exportSchedule"])}>
                {SCHEDULES.map((value) => (
                  <option key={value} value={value} disabled={value !== "off" && !bucketConfigured}>
                    {t.schedules[value].label}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label={t.keep} hint={t.keepHint} error={attempted && invalid ? t.keepError : undefined}>
            {(props) => (
              <div className="w-32">
                <Input
                  {...props}
                  type="number"
                  inputMode="numeric"
                  min={1}
                  max={365}
                  step={1}
                  disabled={!editable}
                  className="tnum"
                  value={keep}
                  onChange={(event) => setKeep(event.target.value)}
                />
              </div>
            )}
          </Field>
          {bucketConfigured ? null : (
            <ReadOnlyNote>{t.bucket}</ReadOnlyNote>
          )}
          {canEdit ? (
            editable ? (
              <div className="flex flex-wrap items-center gap-4">
                <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                  {t.save}
                </Button>
                <ActionError message={failure} />
              </div>
            ) : null
          ) : (
            <ReadOnlyNote>{t.readOnly}</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}

export function ImpersonationForm({ settings, groups, canEdit }: { settings: Settings; groups: Group[]; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const [groupIds, setGroupIds] = useState(() => settings.impersonationGroupIds.filter((id) => groups.some((g) => g.id === id)));
  const dirty = groupIds.length !== settings.impersonationGroupIds.length || groupIds.some((id) => !settings.impersonationGroupIds.includes(id));
  const chosen = groups.filter((g) => settings.impersonationGroupIds.includes(g.id)).map((g) => g.name);
  const { impersonation: t, common } = useMessages(settingsMessages);

  return (
    <Card>
      <CardHeader
        title={t.title}
        description={t.description}
      />
      <CardBody>
        {canEdit ? (
          <form
            noValidate
            className="flex max-w-xl flex-col gap-6"
            onSubmit={(event) => {
              event.preventDefault();
              void save(
                { impersonationGroupIds: groupIds },
                groupIds.length ? t.saved : t.turnedOff,
                groupIds.length ? t.savedHint : t.offHint,
              );
            }}
          >
            <CheckList
              legend={t.legend}
              options={groups.map((g) => ({ id: g.id, name: g.name }))}
              selected={groupIds}
              onToggle={(id, on) => setGroupIds((current) => (on ? [...current, id] : current.filter((v) => v !== id)))}
              empty={common.noGroups}
            />
            <p className="text-caption text-fg-3">{t.hint}</p>
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                {t.save}
              </Button>
              <ActionError message={failure} />
            </div>
          </form>
        ) : (
          <ReadOnlyNote>
            {chosen.length ? t.members(joinList(chosen, common.or)) : t.off} {t.readOnly}
          </ReadOnlyNote>
        )}
      </CardBody>
    </Card>
  );
}

export function SecurityEmailsForm({ settings, templates, canEdit }: { settings: Settings; templates: EmailTemplate[]; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const security = templates.filter((t) => t.security);
  const [off, setOff] = useState(settings.securityEmailsOff);
  const [admins, setAdmins] = useState(settings.lockoutEmailAdmins);
  const dirty = admins !== settings.lockoutEmailAdmins || off.length !== settings.securityEmailsOff.length || off.some((kind) => !settings.securityEmailsOff.includes(kind));
  const turnedOff = security.filter((t) => settings.securityEmailsOff.includes(t.kind)).map((t) => t.label);
  const { securityEmails: copy, common } = useMessages(settingsMessages);

  return (
    <Card>
      <CardHeader
        title={copy.title}
        description={copy.description}
      />
      <CardBody>
        {canEdit ? (
          <form
            noValidate
            className="flex max-w-xl flex-col gap-6"
            onSubmit={(event) => {
              event.preventDefault();
              void save({ securityEmailsOff: off, lockoutEmailAdmins: admins }, copy.saved, common.everyServer);
            }}
          >
            <CheckList
              legend={copy.legend}
              options={security.map((t) => ({ id: t.kind, name: t.label, detail: t.description }))}
              selected={security.filter((t) => !off.includes(t.kind)).map((t) => t.kind)}
              onToggle={(kind, on) => setOff((current) => (on ? current.filter((k) => k !== kind) : [...current, kind]))}
              empty={copy.empty}
              scroll={false}
            />
            <label className="flex cursor-pointer items-start gap-3">
              <Checkbox className="mt-0.5" checked={admins} onChange={(event) => setAdmins(event.target.checked)} />
              <span className="flex flex-col">
                <span className="text-body-sm text-fg">{copy.admins}</span>
                <span className="text-caption text-fg-3">{copy.adminsHint}</span>
              </span>
            </label>
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                {copy.save}
              </Button>
              <ActionError message={failure} />
            </div>
          </form>
        ) : (
          <ReadOnlyNote>
            {turnedOff.length ? copy.notSent(joinList(turnedOff, common.or)) : copy.allSent}
            {settings.lockoutEmailAdmins ? copy.adminsNote : ""} {copy.readOnly}
          </ReadOnlyNote>
        )}
      </CardBody>
    </Card>
  );
}

const SIGN_UP: Settings["signUp"][] = ["off", "open", "approval"];

export function SignUpForm({ settings, groups, domains, canEdit }: { settings: Settings; groups: Group[]; domains: Domain[]; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const assigned = groups.filter((g) => g.kind === "assigned");
  const verified = domains.filter((d) => d.verifiedAt).map((d) => d.name);
  const [mode, setMode] = useState(settings.signUp);
  const [listed, setListed] = useState(settings.signUpDomains.length > 0);
  const [list, setList] = useState(settings.signUpDomains.join(" "));
  const [groupIds, setGroupIds] = useState(() => settings.signUpGroupIds.filter((id) => assigned.some((g) => g.id === id)));
  const [attempted, setAttempted] = useState(false);
  const { signUp: t, common } = useMessages(settingsMessages);
  const domainsChosen = listed ? list.split(/[\s,]+/).filter(Boolean).map((d) => d.toLowerCase()) : [];
  const domainsError =
    mode === "off" ? undefined : listed ? (domainsChosen.length ? undefined : t.domainsRequired) : verified.length ? undefined : t.noVerified;
  const body = { signUp: mode, signUpDomains: domainsChosen, signUpGroupIds: groupIds };
  const dirty = JSON.stringify(body) !== JSON.stringify({ signUp: settings.signUp, signUpDomains: settings.signUpDomains, signUpGroupIds: settings.signUpGroupIds });

  function submit() {
    setAttempted(true);
    if (domainsError) return;
    void save(body, mode === "off" ? t.turnedOff : t.saved, t.savedHint);
  }

  return (
    <Card>
      <CardHeader
        title={t.title}
        description={t.description}
      />
      <CardBody>
        <form
          noValidate
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label={t.who} hint={t.modes[mode].hint}>
            {(props) => (
              <Select {...props} disabled={!canEdit} value={mode} onChange={(event) => setMode(event.target.value as Settings["signUp"])}>
                {SIGN_UP.map((value) => (
                  <option key={value} value={value}>
                    {t.modes[value].label}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          {mode === "off" ? null : (
            <>
              <Field
                label={common.allowedDomains}
                error={attempted && !listed ? domainsError : undefined}
                hint={
                  listed ? undefined : verified.length ? (
                    t.verifiedList(verified.join(", "))
                  ) : (
                    <>
                      {t.verifyBefore}
                      <Link href="/admin/settings/domains" className="text-link underline underline-offset-3">
                        {t.domains}
                      </Link>
                      {t.verifyAfter}
                    </>
                  )
                }
              >
                {(props) => (
                  <Select {...props} disabled={!canEdit} value={listed ? "list" : "verified"} onChange={(event) => setListed(event.target.value === "list")}>
                    <option value="verified">{t.verified}</option>
                    <option value="list">{t.listed}</option>
                  </Select>
                )}
              </Field>
              {listed ? (
                <Field label={t.domains} hint={t.domainsHint} error={attempted ? domainsError : undefined}>
                  {(props) => (
                    <Input {...props} mono autoComplete="off" spellCheck={false} disabled={!canEdit} value={list} onChange={(event) => setList(event.target.value)} placeholder="example.com" />
                  )}
                </Field>
              ) : null}
              <CheckList
                legend={t.addTo}
                options={assigned}
                selected={groupIds}
                onToggle={(id, on) => setGroupIds((current) => (on ? [...current, id] : current.filter((v) => v !== id)))}
                empty={common.noAssignedGroups}
              />
            </>
          )}
          {canEdit ? (
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                {t.save}
              </Button>
              <ActionError message={failure} />
            </div>
          ) : (
            <ReadOnlyNote>{t.readOnly}</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}

const PROVIDERS: Settings["identityProofingProvider"][] = ["", "stripe", "generic"];

export function IdentityProofingForm({
  settings,
  groups,
  issuer,
  keySet,
  webhookSecretSet,
  canEdit,
}: {
  settings: Settings;
  groups: Group[];
  issuer: string;
  keySet: boolean;
  webhookSecretSet: boolean;
  canEdit: boolean;
}) {
  const { saving, failure, save } = useSaveSettings();
  const [invites, setInvites] = useState(settings.identityProofingInvites);
  const [signUps, setSignUps] = useState(settings.identityProofingSignUps);
  const [groupIds, setGroupIds] = useState(() => settings.identityProofingGroupIds.filter((id) => groups.some((g) => g.id === id)));
  const [provider, setProvider] = useState(settings.identityProofingProvider);
  const [name, setName] = useState(settings.identityProofingName);
  const [endpoint, setEndpoint] = useState(settings.identityProofingEndpoint);
  const [apiKey, setApiKey] = useState("");
  const [webhookSecret, setWebhookSecret] = useState("");
  const [attempted, setAttempted] = useState(false);
  const { identityChecks: t, common } = useMessages(settingsMessages);
  const generic = provider === "generic";
  const newProvider = provider !== settings.identityProofingProvider || !keySet;
  const errors = {
    name: generic && (!name.trim() || name.trim().length > 60) ? t.nameError : undefined,
    endpoint: generic && !/^https:\/\/[^\s/@]+/.test(endpoint.trim()) ? t.endpointError : undefined,
    apiKey:
      provider && newProvider && !apiKey.trim()
        ? t.apiKeyError
        : provider === "stripe" && apiKey.trim() && !/^(sk|rk)_/.test(apiKey.trim())
          ? t.stripeKeyError
          : undefined,
    webhookSecret: provider === "stripe" && webhookSecret.trim() && !webhookSecret.trim().startsWith("whsec_") ? t.stripeSecretError : undefined,
  };
  const body = {
    identityProofingInvites: invites,
    identityProofingSignUps: signUps,
    identityProofingGroupIds: groupIds,
    identityProofingProvider: provider,
    identityProofingName: generic ? name.trim() : "",
    identityProofingEndpoint: generic ? endpoint.trim() : "",
  };
  const dirty =
    JSON.stringify(body) !==
      JSON.stringify({
        identityProofingInvites: settings.identityProofingInvites,
        identityProofingSignUps: settings.identityProofingSignUps,
        identityProofingGroupIds: settings.identityProofingGroupIds,
        identityProofingProvider: settings.identityProofingProvider,
        identityProofingName: settings.identityProofingName,
        identityProofingEndpoint: settings.identityProofingEndpoint,
      }) ||
    apiKey.trim() !== "" ||
    webhookSecret.trim() !== "";
  const chosen = groups.filter((g) => settings.identityProofingGroupIds.includes(g.id)).map((g) => g.name);
  const summary =
    settings.identityProofingInvites || settings.identityProofingSignUps
      ? t.summary(settings.identityProofingInvites, settings.identityProofingSignUps, chosen.length ? joinList(chosen, common.or) : "")
      : t.off;

  function submit() {
    setAttempted(true);
    if (Object.values(errors).some(Boolean)) return;
    void save(
      { ...body, ...(apiKey.trim() ? { identityProofingApiKey: apiKey.trim() } : {}), ...(webhookSecret.trim() ? { identityProofingWebhookSecret: webhookSecret.trim() } : {}) },
      invites || signUps ? t.saved : t.turnedOff,
      invites || signUps ? t.savedHint : t.offHint,
    ).then((saved) => {
      if (!saved) return;
      setApiKey("");
      setWebhookSecret("");
    });
  }

  return (
    <Card>
      <CardHeader
        title={t.title}
        description={t.description}
      />
      <CardBody>
        {canEdit ? (
          <form
            noValidate
            className="flex max-w-xl flex-col gap-6"
            onSubmit={(event) => {
              event.preventDefault();
              submit();
            }}
          >
            <fieldset className="flex flex-col gap-3">
              <legend className="mb-3 text-label text-fg">{t.who}</legend>
              <label className="flex cursor-pointer items-start gap-3">
                <Checkbox className="mt-0.5" checked={invites} onChange={(event) => setInvites(event.target.checked)} />
                <span className="flex flex-col">
                  <span className="text-body-sm text-fg">{t.invited}</span>
                  <span className="text-caption text-fg-3">{t.invitedHint}</span>
                </span>
              </label>
              <label className="flex cursor-pointer items-start gap-3">
                <Checkbox className="mt-0.5" checked={signUps} onChange={(event) => setSignUps(event.target.checked)} />
                <span className="flex flex-col">
                  <span className="text-body-sm text-fg">{t.signUps}</span>
                  <span className="text-caption text-fg-3">{t.signUpsHint}</span>
                </span>
              </label>
            </fieldset>
            <CheckList
              legend={t.onlyFor}
              options={groups.map((g) => ({ id: g.id, name: g.name }))}
              selected={groupIds}
              onToggle={(id, on) => setGroupIds((current) => (on ? [...current, id] : current.filter((v) => v !== id)))}
              empty={common.noGroups}
            />
            <p className="text-caption text-fg-3">{t.groupsHint}</p>
            <Field label={t.provider} hint={t.providers[provider].hint}>
              {(props) => (
                <Select {...props} value={provider} onChange={(event) => setProvider(event.target.value as Settings["identityProofingProvider"])}>
                  {PROVIDERS.map((value) => (
                    <option key={value} value={value}>
                      {t.providers[value].label}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            {generic ? (
              <>
                <Field label={t.providerName} hint={t.providerNameHint} error={attempted ? errors.name : undefined}>
                  {(props) => <Input {...props} value={name} onChange={(event) => setName(event.target.value)} placeholder="Acme Verify" />}
                </Field>
                <Field label={t.sessions} hint={t.sessionsHint} error={attempted ? errors.endpoint : undefined}>
                  {(props) => (
                    <Input {...props} mono autoComplete="off" spellCheck={false} value={endpoint} onChange={(event) => setEndpoint(event.target.value)} placeholder="https://verify.example.com/sessions" />
                  )}
                </Field>
              </>
            ) : null}
            {provider ? (
              <>
                <Field
                  label={t.apiKey}
                  hint={keySet && !newProvider ? t.keepKey : common.storedEncrypted}
                  error={attempted ? errors.apiKey : undefined}
                >
                  {(props) => <Input {...props} mono type="password" autoComplete="new-password" spellCheck={false} value={apiKey} onChange={(event) => setApiKey(event.target.value)} />}
                </Field>
                <CopyField
                  label={t.webhook}
                  value={`${issuer}/api/v1/identity-proofing/webhook`}
                  hint={provider === "stripe" ? t.webhookStripe : t.webhookGeneric}
                />
                <Field
                  label={t.webhookSecret}
                  hint={webhookSecretSet ? t.webhookSecretKeep : t.webhookSecretHint}
                  error={attempted ? errors.webhookSecret : undefined}
                >
                  {(props) => (
                    <Input {...props} mono type="password" autoComplete="new-password" spellCheck={false} value={webhookSecret} onChange={(event) => setWebhookSecret(event.target.value)} />
                  )}
                </Field>
              </>
            ) : null}
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                {t.save}
              </Button>
              <ActionError message={failure} />
            </div>
          </form>
        ) : (
          <ReadOnlyNote>{`${summary} ${t.readOnly}`}</ReadOnlyNote>
        )}
      </CardBody>
    </Card>
  );
}
