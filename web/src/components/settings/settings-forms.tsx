"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { CopyField } from "@/components/ui/copy-field";
import { Field, Input, Textarea } from "@/components/ui/input";
import type { Settings } from "@/lib/settings-types";
import { ReadOnlyNote, useSaveSettings } from "./shared";

export function OrganizationForm({ settings, defaultName, issuer, canEdit }: { settings: Settings; defaultName: string; issuer: string; canEdit: boolean }) {
  const { saving, save } = useSaveSettings();
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
            <div>
              <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                Save organization
              </Button>
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
  const { saving, save } = useSaveSettings();
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
            <div>
              <Button type="submit" variant="secondary" loading={saving} disabled={message.trim() === settings.signInMessage}>
                Save message
              </Button>
            </div>
          ) : (
            <ReadOnlyNote>Only a global administrator can change the sign-in message.</ReadOnlyNote>
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
    title: "Code lockout",
    description: "Stops guessing of authenticator app and recovery codes. Passkeys are not affected.",
    fields: [
      { key: "lockoutThreshold", label: "Failed codes before lockout", unit: "codes", min: 3, max: 20, hint: "Between 3 and 20 incorrect codes for the same account." },
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
  const { saving, save } = useSaveSettings();
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
        <div>
          <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
            Save security defaults
          </Button>
        </div>
      ) : (
        <ReadOnlyNote>Only security and global administrators can change security defaults.</ReadOnlyNote>
      )}
    </form>
  );
}
