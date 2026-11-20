"use client";

import { Plus, Trash2, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { ActionError } from "@/components/ui/action-error";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { FieldError } from "@/components/policy/policy-form";
import { Checkbox, Field, Input, Select, Textarea } from "@/components/ui/input";
import type { Settings } from "@/lib/settings-types";
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

type NumberKey = "sessionHours" | "lockoutThreshold" | "lockoutMinutes" | "inviteDays" | "accessTokenMinutes" | "idTokenMinutes" | "refreshTokenHours";

type NumberSpec = { key: NumberKey; label: string; unit: string; min: number; max: number; hint: string };

const GROUPS: { title: string; description: string; fields: NumberSpec[] }[] = [
  {
    title: "Sessions",
    description: "How long someone stays signed in to Halo before signing in again.",
    fields: [{ key: "sessionHours", label: "Session lifetime", unit: "hours", min: 1, max: 72, hint: "Between 1 and 72. Applies to sign-ins from now on; existing sessions keep their expiry." }],
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

export function SecurityForm({ settings, canEdit }: { settings: Settings; canEdit: boolean }) {
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
                        disabled={!canEdit}
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
