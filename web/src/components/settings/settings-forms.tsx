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
import { pluralize } from "@/lib/format";
import type { Domain, EmailTemplate, Settings } from "@/lib/settings-types";
import type { Group } from "@/lib/types";
import { ReadOnlyNote, useSaveSettings } from "./shared";

export function OrganizationForm({ settings, defaultName, issuer, canEdit }: { settings: Settings; defaultName: string; issuer: string; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const [name, setName] = useState(settings.organizationName);
  const [email, setEmail] = useState(settings.contactEmail);
  const [error, setError] = useState<string>();
  const dirty = name.trim() !== settings.organizationName || email.trim() !== settings.contactEmail;

  function submit() {
    if (email.trim() && !/^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/.test(email.trim())) {
      setError("Enter a plain address, such as it@example.com.");
      return;
    }
    setError(undefined);
    void save({ organizationName: name.trim(), contactEmail: email.trim() }, "Organization saved", "The console and sign-in pages show the change within 30 seconds.");
  }

  return (
    <Card>
      <CardHeader title="Profile" description="How Halo names your organization to the people who sign in." />
      <CardBody>
        <form
          noValidate
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label="Organization name" hint={`Shown in the console and on the sign-in and setup pages. Leave empty to use “${defaultName}” from HALO_ORGANIZATION.`}>
            {(props) => <Input {...props} maxLength={100} autoComplete="organization" disabled={!canEdit} value={name} placeholder={defaultName} onChange={(event) => setName(event.target.value)} />}
          </Field>
          <Field label="Contact email" error={error} hint="The sign-in page links to this address under Get help. Leave empty to hide the link.">
            {(props) => <Input {...props} type="email" autoComplete="email" disabled={!canEdit} value={email} placeholder="it@example.com" onChange={(event) => setEmail(event.target.value)} />}
          </Field>
          <CopyField label="Issuer URL" value={issuer} hint="Set by HALO_PUBLIC_URL when Halo starts. Applications discover every protocol endpoint from it." />
          {canEdit ? (
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                Save organization
              </Button>
              <ActionError message={failure} />
            </div>
          ) : (
            <ReadOnlyNote>Only a global administrator can change the organization profile.</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}

export function SignInMessageForm({ settings, canEdit }: { settings: Settings; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const [message, setMessage] = useState(settings.signInMessage);

  return (
    <Card>
      <CardHeader title="Sign-in message" description="A short note under the logo on the sign-in, magic link and setup pages, such as where to get help." />
      <CardBody>
        <form
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            void save({ signInMessage: message.trim() }, message.trim() ? "Sign-in message saved" : "Sign-in message removed", "The sign-in page shows the change within 30 seconds.");
          }}
        >
          <Field label="Message" hint={`${message.length} of 500 characters. Plain text; line breaks are kept.`}>
            {(props) => (
              <Textarea {...props} rows={4} maxLength={500} disabled={!canEdit} value={message} placeholder="Use the passkey on your work laptop. Lost it? Call the IT desk on extension 4000." onChange={(event) => setMessage(event.target.value)} />
            )}
          </Field>
          {canEdit ? (
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="secondary" loading={saving} disabled={message.trim() === settings.signInMessage}>
                Save message
              </Button>
              <ActionError message={failure} />
            </div>
          ) : (
            <ReadOnlyNote>Only a global administrator can change the sign-in message.</ReadOnlyNote>
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
  const bytes = new TextEncoder().encode(css.trim()).length;
  const error =
    bytes > CSS_LIMIT
      ? `Remove ${(bytes - CSS_LIMIT).toLocaleString("en")} bytes to save it.`
      : css.includes("</")
        ? "Remove “</”, which would end the style element. In a CSS string, write \\3c/ instead."
        : undefined;

  return (
    <Card>
      <CardHeader
        title="Custom CSS"
        description="Added after Halo's stylesheet on the sign-in, setup and account pages, for what the colours don't cover, such as fonts, corner radius or spacing."
      />
      <CardBody>
        <form
          className="flex flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            void save({ customCss: css.trim(), consoleBranding: inConsole }, "Custom CSS saved", "The sign-in, setup and account pages use it within 30 seconds.");
          }}
        >
          {disabled ? (
            <div role="note" className="flex gap-3 rounded-md border border-warning/20 bg-warning-container p-4">
              <TriangleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0 text-warning" />
              <p className="text-body-sm text-fg-2">
                This server runs with <code className="text-code-sm">HALO_DISABLE_CUSTOM_CSS=1</code>, so no page uses the custom CSS. Fix it here, then remove the variable and restart Halo.
              </p>
            </div>
          ) : null}
          <Field label="CSS" error={error} hint={`${bytes.toLocaleString("en")} of 32,768 bytes. Images, fonts and imports load only from this Halo server.`}>
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
              <span className="text-body-sm text-fg">Use the colours and custom CSS in the admin console too</span>
              <span className="text-caption text-fg-3">Leave it off while you try out a stylesheet, so a mistake can't hide the controls you need to undo it.</span>
            </span>
          </label>
          {canEdit ? (
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="secondary" loading={saving} disabled={Boolean(error) || (css.trim() === settings.customCss && inConsole === settings.consoleBranding)}>
                Save custom CSS
              </Button>
              <ActionError message={failure} />
            </div>
          ) : (
            <ReadOnlyNote>Only a global administrator can change the custom CSS.</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}

type NumberKey = "sessionHours" | "reauthMinutes" | "lockoutThreshold" | "lockoutMinutes" | "inviteDays" | "accessTokenMinutes" | "idTokenMinutes" | "refreshTokenHours";

type NumberSpec = { key: NumberKey; label: string; unit: string; min: number; max: number; hint: string; global?: boolean };

const GROUPS: { title: string; description: string; fields: NumberSpec[] }[] = [
  {
    title: "Sessions",
    description: "How long someone stays signed in to Halo, and how recent their last passkey sign-in must be for sensitive changes.",
    fields: [
      { key: "sessionHours", label: "Session lifetime", unit: "hours", min: 1, max: 72, hint: "Between 1 and 72. Applies to sign-ins from now on; existing sessions keep their expiry." },
      {
        key: "reauthMinutes",
        label: "Confirm identity after",
        unit: "minutes",
        min: 1,
        max: 1440,
        hint: "Between 1 and 1,440. Granting roles, creating API keys and other sensitive changes ask for a passkey again after this long. Only a global administrator can change it.",
        global: true,
      },
    ],
  },
  {
    title: "Lockout",
    description: "Stops guessing of passwords, authenticator app codes and recovery codes. Passkeys are not affected.",
    fields: [
      { key: "lockoutThreshold", label: "Failed attempts before lockout", unit: "attempts", min: 3, max: 20, hint: "Between 3 and 20 incorrect passwords or codes for the same account." },
      { key: "lockoutMinutes", label: "Lockout window", unit: "minutes", min: 5, max: 1440, hint: "Between 5 and 1,440. Failures older than this stop counting." },
    ],
  },
  {
    title: "Setup links",
    description: "Invitation and reset links that let someone enroll a passkey.",
    fields: [{ key: "inviteDays", label: "Setup link lifetime", unit: "days", min: 1, max: 30, hint: "Between 1 and 30. Links already sent keep their expiry." }],
  },
  {
    title: "Defaults for new applications",
    description: "Token lifetimes given to applications registered from now on. Existing applications keep their own.",
    fields: [
      { key: "accessTokenMinutes", label: "Access token", unit: "minutes", min: 5, max: 1440, hint: "Between 5 and 1,440." },
      { key: "idTokenMinutes", label: "ID token", unit: "minutes", min: 5, max: 1440, hint: "Between 5 and 1,440." },
      { key: "refreshTokenHours", label: "Refresh token", unit: "hours", min: 1, max: 2160, hint: "Between 1 and 2,160 (90 days)." },
    ],
  },
];

const FIELDS = GROUPS.flatMap((group) => group.fields);

export function SecurityForm({ settings, canEdit, canEditGlobal }: { settings: Settings; canEdit: boolean; canEditGlobal: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const [values, setValues] = useState(() => Object.fromEntries(FIELDS.map((f) => [f.key, String(settings[f.key])])) as Record<NumberKey, string>);
  const [errors, setErrors] = useState<Partial<Record<NumberKey, string>>>({});
  const dirty = FIELDS.some((f) => Number(values[f.key]) !== settings[f.key]);

  function submit() {
    const next: Partial<Record<NumberKey, string>> = {};
    for (const f of FIELDS) {
      const value = Number(values[f.key]);
      if (!Number.isInteger(value) || value < f.min || value > f.max) next[f.key] = `Enter a whole number between ${f.min.toLocaleString("en")} and ${f.max.toLocaleString("en")}.`;
    }
    setErrors(next);
    if (Object.keys(next).length) return;
    const body = Object.fromEntries(FIELDS.map((f) => [f.key, Number(values[f.key])]));
    void save(body, "Security defaults saved", "New sessions, codes, setup links and applications use them within 30 seconds.");
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
        <Card key={group.title}>
          <CardHeader title={group.title} description={group.description} />
          <CardBody className="grid grid-cols-1 gap-6 md:grid-cols-3">
            {group.fields.map((f) => (
              <Field key={f.key} label={f.label} hint={f.hint} error={errors[f.key]}>
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
                    <span className="text-body-sm text-fg-3">{f.unit}</span>
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
            Save security defaults
          </Button>
          <ActionError message={failure} />
        </div>
      ) : (
        <ReadOnlyNote>Only security and global administrators can change security defaults.</ReadOnlyNote>
      )}
    </form>
  );
}

const REGISTRATION: Record<Settings["clientRegistration"], { label: string; hint: string }> = {
  off: { label: "Nobody", hint: "Only administrators register applications, in the console or with the API." },
  token: { label: "Clients with an initial access token", hint: "Clients send an API key with the Client registration scope. Create one on a service account and give it to the tools you trust." },
  approval: { label: "Anyone, after approval", hint: "Anyone who can reach Halo can register a client. It stays disabled until an application administrator approves it." },
  open: { label: "Anyone", hint: "Anyone who can reach Halo can register a client. It still signs nobody in until you assign it to groups." },
};

export function ClientRegistrationForm({ settings, issuer, canEdit }: { settings: Settings; issuer: string; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const [mode, setMode] = useState(settings.clientRegistration);

  return (
    <Card>
      <CardHeader
        title="Client registration"
        description="Lets OAuth clients register themselves (RFC 7591) and change or delete their registration with the access token they receive (RFC 7592). They get only the openid, profile and email scopes, and never groups, API grants or token exchange. A client assigned to groups that changes its registration waits for approval again."
      />
      <CardBody>
        <form
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            void save({ clientRegistration: mode }, mode === "off" ? "Client registration turned off" : "Client registration saved", "Every Halo server follows the change within 30 seconds.");
          }}
        >
          <Field label="Who can register clients" hint={REGISTRATION[mode].hint}>
            {(props) => (
              <Select {...props} disabled={!canEdit} value={mode} onChange={(event) => setMode(event.target.value as Settings["clientRegistration"])}>
                {(Object.keys(REGISTRATION) as Settings["clientRegistration"][]).map((value) => (
                  <option key={value} value={value}>
                    {REGISTRATION[value].label}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          {mode === "off" ? null : <CopyField label="Registration endpoint" value={`${issuer}/oauth2/register`} hint="Clients also find it as registration_endpoint in the discovery document." />}
          {canEdit ? (
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={saving} disabled={mode === settings.clientRegistration}>
                Save client registration
              </Button>
              <ActionError message={failure} />
            </div>
          ) : (
            <ReadOnlyNote>Only security and global administrators can change client registration.</ReadOnlyNote>
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
  const limitError = sessionCount(limit, 0) ? undefined : "Enter a whole number between 0 and 100.";
  const rowsError = rows.some((row) => !row.groupId)
    ? "Choose a group for every group limit, or remove the empty one."
    : rows.some((row) => !sessionCount(row.limit, 1))
      ? "Give each group a whole number of sessions between 1 and 100."
      : undefined;
  const body = { sessionLimit: Number(limit), sessionLimitAction: action, sessionLimitCli: cli, sessionLimitGroups: rows.map((row) => ({ groupId: row.groupId, limit: Number(row.limit) })) };
  const saved = { sessionLimit: settings.sessionLimit, sessionLimitAction: settings.sessionLimitAction, sessionLimitCli: settings.sessionLimitCli, sessionLimitGroups: settings.sessionLimitGroups };
  const dirty = JSON.stringify(body) !== JSON.stringify(saved);
  const groupName = (id: string) => groups.find((g) => g.id === id)?.name ?? "this group";

  function update(index: number, change: Partial<(typeof rows)[number]>) {
    setRows((current) => current.map((row, i) => (i === index ? { ...row, ...change } : row)));
  }

  function submit() {
    setAttempted(true);
    if (limitError || rowsError) return;
    void save(body, "Session limit saved", "New sign-ins follow it within 30 seconds. Existing sessions stay until someone signs in again.");
  }

  return (
    <Card>
      <CardHeader title="Session limit" description="The most sessions one person can hold at once. A limit discourages shared accounts and stops a stolen session from being used quietly alongside the real one." />
      <CardBody>
        <form
          noValidate
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label="Sessions per person" hint="Between 1 and 100, or 0 for no limit. Checked at every new sign-in." error={attempted ? limitError : undefined}>
            {(props) => (
              <div className="flex items-center gap-3">
                <div className="w-32">
                  <Input {...props} type="number" inputMode="numeric" min={0} max={100} step={1} disabled={!canEdit} className="tnum" value={limit} onChange={(event) => setLimit(event.target.value)} />
                </div>
                <span className="text-body-sm text-fg-3">sessions</span>
              </div>
            )}
          </Field>
          <Field label="When someone reaches the limit" hint="Refused sign-ins and ended sessions are recorded in the sign-in log.">
            {(props) => (
              <Select {...props} disabled={!canEdit} value={action} onChange={(event) => setAction(event.target.value as Settings["sessionLimitAction"])}>
                <option value="refuse">Refuse the new sign-in</option>
                <option value="end-oldest">End their oldest session</option>
              </Select>
            )}
          </Field>
          <label className="flex cursor-pointer items-start gap-3">
            <Checkbox className="mt-0.5" disabled={!canEdit} checked={cli} onChange={(event) => setCli(event.target.checked)} />
            <span className="flex flex-col">
              <span className="text-body-sm text-fg">Also limit Halo CLI sessions</span>
              <span className="text-caption text-fg-3">
                Counted apart from browser sessions, against the same limit, when someone approves <code className="text-code-sm">halo login</code>. When off, the command line is never limited.
              </span>
            </span>
          </label>
          <fieldset className="flex flex-col gap-3">
            <legend className="mb-1 text-label text-fg">Group limits</legend>
            <p className="text-caption text-fg-3">Replace the organization&apos;s limit for members of a group. Someone in several of these groups gets the lowest of their limits.</p>
            {rows.map((row, index) => (
              <div key={index} className="flex items-center gap-3">
                <Select aria-label={`Group ${index + 1}`} disabled={!canEdit} value={row.groupId} onChange={(event) => update(index, { groupId: event.target.value })}>
                  <option value="">Choose a group</option>
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
                    aria-label={`Sessions for ${groupName(row.groupId)}`}
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
                  <Button size="icon-sm" variant="quiet" aria-label={`Remove the limit for ${groupName(row.groupId)}`} onClick={() => setRows((current) => current.filter((_, i) => i !== index))}>
                    <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                  </Button>
                ) : null}
              </div>
            ))}
            {canEdit ? (
              <div>
                <Button size="sm" variant="secondary" disabled={rows.length >= groups.length} onClick={() => setRows((current) => [...current, { groupId: "", limit: "1" }])}>
                  <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
                  Add group limit
                </Button>
              </div>
            ) : null}
            <FieldError>{attempted ? rowsError : undefined}</FieldError>
          </fieldset>
          {canEdit ? (
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                Save session limit
              </Button>
              <ActionError message={failure} />
            </div>
          ) : (
            <ReadOnlyNote>Only security and global administrators can change the session limit.</ReadOnlyNote>
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
  const siteKeyError = provider && !/^[A-Za-z0-9_-]{1,100}$/.test(siteKey.trim()) ? "Enter the site key from your provider's dashboard." : undefined;
  const secretError = provider && !secretSet && !secret.trim() ? "Enter the secret key from your provider's dashboard." : undefined;
  const dirty = provider !== settings.botProtection || siteKey.trim() !== settings.botProtectionSiteKey || secret.trim() !== "";

  function submit() {
    setAttempted(true);
    if (siteKeyError || secretError) return;
    void save(
      { botProtection: provider, botProtectionSiteKey: siteKey.trim(), ...(secret.trim() ? { botProtectionSecret: secret.trim() } : {}) },
      provider ? "Bot protection saved" : "Bot protection turned off",
      "The sign-in page follows it within 30 seconds.",
    ).then((saved) => saved && setSecret(""));
  }

  return (
    <Card>
      <CardHeader
        title="Bot protection"
        description="Asks for a check from Cloudflare Turnstile or hCaptcha before Halo emails a sign-in link, a code or a sign-up link, and before passwords and codes from an IP address that reached the lockout threshold. Passkeys are never checked."
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
          <Field label="Provider" hint="Halo verifies every check with the provider. If the provider can't be reached, these requests are refused until it can.">
            {(props) => (
              <Select {...props} disabled={!canEdit} value={provider} onChange={(event) => setProvider(event.target.value as Settings["botProtection"])}>
                <option value="">Off</option>
                <option value="turnstile">Cloudflare Turnstile</option>
                <option value="hcaptcha">hCaptcha</option>
              </Select>
            )}
          </Field>
          {provider ? (
            <>
              <Field label="Site key" hint="Public. The sign-in page uses it to show the check." error={attempted ? siteKeyError : undefined}>
                {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} disabled={!canEdit} value={siteKey} onChange={(event) => setSiteKey(event.target.value)} />}
              </Field>
              <Field
                label="Secret key"
                hint={secretSet ? "Leave empty to keep the current secret. Halo stores it encrypted and never shows it again." : "Stored encrypted. Halo never shows it again."}
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
                Save bot protection
              </Button>
              <ActionError message={failure} />
            </div>
          ) : (
            <ReadOnlyNote>Only security and global administrators can change bot protection.</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}

type RetentionKey = "signInLogDays" | "auditLogDays";

const RETENTION: { key: RetentionKey; label: string; min: number; hint: string }[] = [
  { key: "signInLogDays", label: "Sign-in log", min: 7, hint: "Between 7 and 3,650, or 0 to keep entries forever. Entries hold IP addresses, locations and devices." },
  { key: "auditLogDays", label: "Audit log", min: 30, hint: "Between 30 and 3,650, or 0 to keep entries forever." },
];

export function LogRetentionForm({ settings, canEdit }: { settings: Settings; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const [values, setValues] = useState({ signInLogDays: String(settings.signInLogDays), auditLogDays: String(settings.auditLogDays) });
  const [attempted, setAttempted] = useState(false);
  const days = (key: RetentionKey) => Number(values[key]);
  const invalid = (f: (typeof RETENTION)[number]) => values[f.key].trim() === "" || !Number.isInteger(days(f.key)) || (days(f.key) !== 0 && (days(f.key) < f.min || days(f.key) > 3650));
  const dirty = RETENTION.some((f) => days(f.key) !== settings[f.key]);
  const shortened = RETENTION.some((f) => !invalid(f) && days(f.key) > 0 && (settings[f.key] === 0 || days(f.key) < settings[f.key]));

  function submit() {
    setAttempted(true);
    if (RETENTION.some(invalid)) return;
    void save({ signInLogDays: days("signInLogDays"), auditLogDays: days("auditLogDays") }, "Log retention saved", "Halo applies it at its next daily cleanup.");
  }

  return (
    <Card>
      <CardHeader title="Log retention" description="How long Halo keeps sign-in and audit log entries. Once a day it deletes older entries and records how many in the audit log." />
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
            <Field key={f.key} label={f.label} hint={f.hint} error={attempted && invalid(f) ? `Enter a whole number between ${f.min} and 3,650, or 0.` : undefined}>
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
                  <span className="text-body-sm text-fg-3">days</span>
                </div>
              )}
            </Field>
          ))}
          {shortened ? (
            <div role="note" className="flex gap-3 rounded-md border border-warning/20 bg-warning-container p-4">
              <TriangleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0 text-warning" />
              <p className="text-body-sm text-fg-2">Entries older than the new period are deleted within a day of saving. Halo can&apos;t bring them back.</p>
            </div>
          ) : null}
          {canEdit ? (
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                Save log retention
              </Button>
              <ActionError message={failure} />
            </div>
          ) : (
            <ReadOnlyNote>Only a global administrator can change log retention.</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}

const SCHEDULES: Record<Settings["exportSchedule"], { label: string; hint: string }> = {
  off: { label: "Off", hint: "Halo saves no exports on its own. Download one above whenever you need it." },
  daily: { label: "Every day", hint: "Halo saves the export once a day and records it in the audit log." },
  weekly: { label: "Every week", hint: "Halo saves the export once a week and records it in the audit log." },
};

export function ScheduledExportForm({ settings, bucketConfigured, canEdit }: { settings: Settings; bucketConfigured: boolean; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const [schedule, setSchedule] = useState(settings.exportSchedule);
  const [keep, setKeep] = useState(String(settings.exportKeep));
  const [attempted, setAttempted] = useState(false);
  const count = Number(keep);
  const invalid = keep.trim() === "" || !Number.isInteger(count) || count < 1 || count > 365;
  const dirty = schedule !== settings.exportSchedule || count !== settings.exportKeep;
  const editable = canEdit && (bucketConfigured || settings.exportSchedule !== "off");

  function submit() {
    setAttempted(true);
    if (invalid) return;
    void save({ exportSchedule: schedule, exportKeep: count }, schedule === "off" ? "Scheduled exports turned off" : "Scheduled exports saved", `Halo keeps the last ${pluralize(count, "export")} in the bucket.`);
  }

  return (
    <Card>
      <CardHeader title="Scheduled exports" description="Save the same JSON export to your S3-compatible bucket on a schedule, as exports/halo-export-<time>.json, so a recent copy is there to import." />
      <CardBody>
        <form
          noValidate
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label="Schedule" hint={SCHEDULES[schedule].hint}>
            {(props) => (
              <Select {...props} disabled={!editable} value={schedule} onChange={(event) => setSchedule(event.target.value as Settings["exportSchedule"])}>
                {(Object.keys(SCHEDULES) as Settings["exportSchedule"][]).map((value) => (
                  <option key={value} value={value} disabled={value !== "off" && !bucketConfigured}>
                    {SCHEDULES[value].label}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Exports to keep" hint="Between 1 and 365. After each new export, Halo deletes the oldest ones." error={attempted && invalid ? "Enter a whole number between 1 and 365." : undefined}>
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
            <ReadOnlyNote>Set HALO_S3_ENDPOINT, HALO_S3_BUCKET, HALO_S3_ACCESS_KEY_ID and HALO_S3_SECRET_ACCESS_KEY to save scheduled exports to a bucket.</ReadOnlyNote>
          )}
          {canEdit ? (
            editable ? (
              <div className="flex flex-wrap items-center gap-4">
                <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                  Save scheduled exports
                </Button>
                <ActionError message={failure} />
              </div>
            ) : null
          ) : (
            <ReadOnlyNote>Only a global administrator can change scheduled exports.</ReadOnlyNote>
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

  return (
    <Card>
      <CardHeader
        title="Impersonation"
        description="Lets helpdesk administrators see Halo as someone for 30 minutes to find out why their access fails. They can look but not change anything or open applications, the person gets an email unless it is turned off under Security emails, and both names go in the audit and sign-in logs. People who hold or are eligible for an administrator role can't be impersonated."
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
                groupIds.length ? "Impersonation saved" : "Impersonation turned off",
                groupIds.length ? "Helpdesk administrators can impersonate members of these groups within 30 seconds." : "Nobody can be impersonated. Impersonations already running continue until they end.",
              );
            }}
          >
            <CheckList
              legend="Whose accounts can be impersonated"
              options={groups.map((g) => ({ id: g.id, name: g.name }))}
              selected={groupIds}
              onToggle={(id, on) => setGroupIds((current) => (on ? [...current, id] : current.filter((v) => v !== id)))}
              empty="There are no groups yet."
            />
            <p className="text-caption text-fg-3">Choose no group to turn impersonation off. To have a second global administrator approve each one first, turn on approval for impersonation under Approvals.</p>
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                Save impersonation
              </Button>
              <ActionError message={failure} />
            </div>
          </form>
        ) : (
          <ReadOnlyNote>
            {chosen.length ? `Members of ${joinList(chosen)} can be impersonated.` : "Impersonation is off."} Only global administrators can change who can be impersonated.
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

  return (
    <Card>
      <CardHeader
        title="Security emails"
        description="Emails Halo sends people about their own account, so they notice a takeover early. They link to the account portal, never to a sign-in page, and never contain a code. Halo sends one email of a kind about the same thing within 15 minutes, and one lockout email per lockout window. Change their text under Email templates."
      />
      <CardBody>
        {canEdit ? (
          <form
            noValidate
            className="flex max-w-xl flex-col gap-6"
            onSubmit={(event) => {
              event.preventDefault();
              void save({ securityEmailsOff: off, lockoutEmailAdmins: admins }, "Security emails saved", "Every Halo server follows the change within 30 seconds.");
            }}
          >
            <CheckList
              legend="Emails people get"
              options={security.map((t) => ({ id: t.kind, name: t.label, detail: t.description }))}
              selected={security.filter((t) => !off.includes(t.kind)).map((t) => t.kind)}
              onToggle={(kind, on) => setOff((current) => (on ? current.filter((k) => k !== kind) : [...current, kind]))}
              empty="Halo has no security emails to choose from."
              scroll={false}
            />
            <label className="flex cursor-pointer items-start gap-3">
              <Checkbox className="mt-0.5" checked={admins} onChange={(event) => setAdmins(event.target.checked)} />
              <span className="flex flex-col">
                <span className="text-body-sm text-fg">Also email global administrators when an account is locked</span>
                <span className="text-caption text-fg-3">With the person&apos;s name and a link to their page in the console, once per lockout window.</span>
              </span>
            </label>
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                Save security emails
              </Button>
              <ActionError message={failure} />
            </div>
          </form>
        ) : (
          <ReadOnlyNote>
            {turnedOff.length ? `Halo doesn't send these emails: ${joinList(turnedOff)}.` : "Halo sends every security email."}
            {settings.lockoutEmailAdmins ? " Global administrators are emailed when an account is locked." : ""} Only security and global administrators can change security emails.
          </ReadOnlyNote>
        )}
      </CardBody>
    </Card>
  );
}

const SIGN_UP: Record<Settings["signUp"], { label: string; hint: string }> = {
  off: { label: "Nobody", hint: "People join only by invitation, SCIM or an identity provider." },
  open: { label: "Allowed domains, no approval", hint: "Their account is active once they confirm their email address and create a passkey." },
  approval: { label: "Allowed domains, with approval", hint: "After they confirm their email address and create a passkey, a user administrator approves them on the Users page." },
};

export function SignUpForm({ settings, groups, domains, canEdit }: { settings: Settings; groups: Group[]; domains: Domain[]; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const assigned = groups.filter((g) => g.kind === "assigned");
  const verified = domains.filter((d) => d.verifiedAt).map((d) => d.name);
  const [mode, setMode] = useState(settings.signUp);
  const [listed, setListed] = useState(settings.signUpDomains.length > 0);
  const [list, setList] = useState(settings.signUpDomains.join(" "));
  const [groupIds, setGroupIds] = useState(() => settings.signUpGroupIds.filter((id) => assigned.some((g) => g.id === id)));
  const [attempted, setAttempted] = useState(false);
  const domainsChosen = listed ? list.split(/[\s,]+/).filter(Boolean).map((d) => d.toLowerCase()) : [];
  const domainsError =
    mode === "off" ? undefined : listed ? (domainsChosen.length ? undefined : "Enter at least one domain, such as example.com.") : verified.length ? undefined : "No domain is verified yet, so nobody could sign up.";
  const body = { signUp: mode, signUpDomains: domainsChosen, signUpGroupIds: groupIds };
  const dirty = JSON.stringify(body) !== JSON.stringify({ signUp: settings.signUp, signUpDomains: settings.signUpDomains, signUpGroupIds: settings.signUpGroupIds });

  function submit() {
    setAttempted(true);
    if (domainsError) return;
    void save(body, mode === "off" ? "Sign-up turned off" : "Sign-up saved", "The sign-up page follows it within 30 seconds.");
  }

  return (
    <Card>
      <CardHeader
        title="Sign-up"
        description="Lets people create their own account at /sign-up with their email address and a passkey. New accounts get no administrator roles. Bot protection, when on, checks every sign-up, and each network can sign up 10 times an hour."
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
          <Field label="Who can sign up" hint={SIGN_UP[mode].hint}>
            {(props) => (
              <Select {...props} disabled={!canEdit} value={mode} onChange={(event) => setMode(event.target.value as Settings["signUp"])}>
                {(Object.keys(SIGN_UP) as Settings["signUp"][]).map((value) => (
                  <option key={value} value={value}>
                    {SIGN_UP[value].label}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          {mode === "off" ? null : (
            <>
              <Field
                label="Allowed email domains"
                error={attempted && !listed ? domainsError : undefined}
                hint={
                  listed ? undefined : verified.length ? (
                    `Verified: ${verified.join(", ")}.`
                  ) : (
                    <>
                      Verify a domain under{" "}
                      <Link href="/admin/settings/domains" className="text-link underline underline-offset-3">
                        Domains
                      </Link>{" "}
                      first, or list the domains.
                    </>
                  )
                }
              >
                {(props) => (
                  <Select {...props} disabled={!canEdit} value={listed ? "list" : "verified"} onChange={(event) => setListed(event.target.value === "list")}>
                    <option value="verified">Verified domains</option>
                    <option value="list">These domains</option>
                  </Select>
                )}
              </Field>
              {listed ? (
                <Field label="Domains" hint="Separated by spaces. Each must match the part of the address after the @ exactly." error={attempted ? domainsError : undefined}>
                  {(props) => (
                    <Input {...props} mono autoComplete="off" spellCheck={false} disabled={!canEdit} value={list} onChange={(event) => setList(event.target.value)} placeholder="example.com" />
                  )}
                </Field>
              ) : null}
              <CheckList
                legend="Add new accounts to"
                options={assigned}
                selected={groupIds}
                onToggle={(id, on) => setGroupIds((current) => (on ? [...current, id] : current.filter((v) => v !== id)))}
                empty="No assigned groups yet. Dynamic groups pick people up from their profile."
              />
            </>
          )}
          {canEdit ? (
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                Save sign-up
              </Button>
              <ActionError message={failure} />
            </div>
          ) : (
            <ReadOnlyNote>Only security and global administrators can change sign-up.</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}

const PROVIDERS: Record<Settings["identityProofingProvider"], { label: string; hint: string }> = {
  "": { label: "In person only", hint: "A user or helpdesk administrator checks each person and records it on their page or in the sign-up queue." },
  stripe: { label: "Stripe Identity", hint: "People photograph an identity document and their face on Stripe's page. Administrators can still check people in person." },
  generic: { label: "Another provider", hint: "Any service that follows Halo's session and webhook contract, described in the API reference under the identity proofing webhook." },
};

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
  const generic = provider === "generic";
  const newProvider = provider !== settings.identityProofingProvider || !keySet;
  const errors = {
    name: generic && (!name.trim() || name.trim().length > 60) ? "Name the provider in up to 60 characters. People see it on their setup page." : undefined,
    endpoint: generic && !/^https:\/\/[^\s/@]+/.test(endpoint.trim()) ? "Enter the provider's sessions address as an https URL." : undefined,
    apiKey:
      provider && newProvider && !apiKey.trim()
        ? "Enter the API key from the provider."
        : provider === "stripe" && apiKey.trim() && !/^(sk|rk)_/.test(apiKey.trim())
          ? "Enter a Stripe secret key or restricted key, which starts with sk_ or rk_."
          : undefined,
    webhookSecret: provider === "stripe" && webhookSecret.trim() && !webhookSecret.trim().startsWith("whsec_") ? "Enter the signing secret of the Stripe webhook endpoint, which starts with whsec_." : undefined,
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
  const who = [settings.identityProofingInvites ? "Invited people" : "", settings.identityProofingSignUps ? "people who sign up" : ""].filter(Boolean).join(" and ");
  const summary = who ? `${who.charAt(0).toUpperCase()}${who.slice(1)}${chosen.length ? ` in ${joinList(chosen)}` : ""} need an identity check before their account is activated.` : "Identity checks are off.";

  function submit() {
    setAttempted(true);
    if (Object.values(errors).some(Boolean)) return;
    void save(
      { ...body, ...(apiKey.trim() ? { identityProofingApiKey: apiKey.trim() } : {}), ...(webhookSecret.trim() ? { identityProofingWebhookSecret: webhookSecret.trim() } : {}) },
      invites || signUps ? "Identity checks saved" : "Identity checks turned off",
      invites || signUps ? "Accounts that need a check stay inactive until one succeeds. Setup links follow within 30 seconds." : "Accounts activate without an identity check within 30 seconds.",
    ).then((saved) => {
      if (!saved) return;
      setApiKey("");
      setWebhookSecret("");
    });
  }

  return (
    <Card>
      <CardHeader
        title="Identity checks"
        description="Keeps an account inactive until someone confirms the person is who the invitation or sign-up names. Halo stores only how and when they were checked and the result, never the details from their document."
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
              <legend className="mb-3 text-label text-fg">Who needs a check</legend>
              <label className="flex cursor-pointer items-start gap-3">
                <Checkbox className="mt-0.5" checked={invites} onChange={(event) => setInvites(event.target.checked)} />
                <span className="flex flex-col">
                  <span className="text-body-sm text-fg">Invited people</span>
                  <span className="text-caption text-fg-3">Their setup link and every way to sign in stay closed until the check succeeds. People who are already active aren&apos;t affected.</span>
                </span>
              </label>
              <label className="flex cursor-pointer items-start gap-3">
                <Checkbox className="mt-0.5" checked={signUps} onChange={(event) => setSignUps(event.target.checked)} />
                <span className="flex flex-col">
                  <span className="text-body-sm text-fg">People who sign up</span>
                  <span className="text-caption text-fg-3">Their sign-up waits in the queue on the Users page until the check succeeds, even when anyone at an allowed domain can sign up.</span>
                </span>
              </label>
            </fieldset>
            <CheckList
              legend="Only for members of"
              options={groups.map((g) => ({ id: g.id, name: g.name }))}
              selected={groupIds}
              onToggle={(id, on) => setGroupIds((current) => (on ? [...current, id] : current.filter((v) => v !== id)))}
              empty="There are no groups yet."
            />
            <p className="text-caption text-fg-3">Choose no group to check everyone above. Sign-ups count as members of the groups new accounts join.</p>
            <Field label="Verification provider" hint={PROVIDERS[provider].hint}>
              {(props) => (
                <Select {...props} value={provider} onChange={(event) => setProvider(event.target.value as Settings["identityProofingProvider"])}>
                  {(Object.keys(PROVIDERS) as Settings["identityProofingProvider"][]).map((value) => (
                    <option key={value} value={value}>
                      {PROVIDERS[value].label}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            {generic ? (
              <>
                <Field label="Provider name" hint="People see it on their setup page." error={attempted ? errors.name : undefined}>
                  {(props) => <Input {...props} value={name} onChange={(event) => setName(event.target.value)} placeholder="Acme Verify" />}
                </Field>
                <Field label="Sessions address" hint="Halo creates a verification session here for each check." error={attempted ? errors.endpoint : undefined}>
                  {(props) => (
                    <Input {...props} mono autoComplete="off" spellCheck={false} value={endpoint} onChange={(event) => setEndpoint(event.target.value)} placeholder="https://verify.example.com/sessions" />
                  )}
                </Field>
              </>
            ) : null}
            {provider ? (
              <>
                <Field
                  label="API key"
                  hint={keySet && !newProvider ? "Leave empty to keep the current key. Halo stores it encrypted and never shows it again." : "Stored encrypted. Halo never shows it again."}
                  error={attempted ? errors.apiKey : undefined}
                >
                  {(props) => <Input {...props} mono type="password" autoComplete="new-password" spellCheck={false} value={apiKey} onChange={(event) => setApiKey(event.target.value)} />}
                </Field>
                <CopyField
                  label="Webhook address"
                  value={`${issuer}/api/v1/identity-proofing/webhook`}
                  hint={provider === "stripe" ? "Add it as a webhook endpoint in Stripe for the identity.verification_session events." : "Give it to the provider for result notifications."}
                />
                <Field
                  label="Webhook signing secret"
                  hint={
                    webhookSecretSet
                      ? "Leave empty to keep the current secret."
                      : "Optional. Without it, Halo refuses webhooks and asks the provider for each result instead, when the person returns and every 10 minutes."
                  }
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
                Save identity checks
              </Button>
              <ActionError message={failure} />
            </div>
          </form>
        ) : (
          <ReadOnlyNote>{`${summary} Only security and global administrators can change identity checks.`}</ReadOnlyNote>
        )}
      </CardBody>
    </Card>
  );
}
