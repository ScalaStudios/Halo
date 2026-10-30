"use client";

import { TriangleAlert } from "lucide-react";
import { useState } from "react";
import { ActionError } from "@/components/ui/action-error";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { Checkbox, Field, Input, Textarea } from "@/components/ui/input";
import type { Settings } from "@/lib/settings-types";
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
