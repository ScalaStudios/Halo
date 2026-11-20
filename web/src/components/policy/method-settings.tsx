"use client";

import { Plus, Trash2, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { SecretWarning } from "@/components/applications/shared";
import { useMutation } from "@/components/console/use-mutation";
import { useSaveSettings } from "@/components/settings/shared";
import { ActionError } from "@/components/ui/action-error";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { Checkbox, Field, Input, Select, Textarea } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { api } from "@/lib/api/client";
import { pluralize } from "@/lib/format";
import { METHOD } from "@/lib/labels";
import type { MethodSetting } from "@/lib/policy-types";
import type { Settings } from "@/lib/settings-types";
import type { MethodKind } from "@/lib/types";
import { CheckList, FieldError, type Option } from "./policy-form";
import { METHOD_PLURAL } from "./shared";

const DESCRIPTION: Record<MethodKind, string> = {
  passkey: "Face ID, Touch ID, Windows Hello or a phone screen lock. Bound to this domain, so a fake sign-in page can't use them.",
  "security-key": "Hardware keys such as a YubiKey or Titan Key. Phishing-resistant and portable between computers.",
  totp: "Six-digit codes from apps such as 1Password, Google Authenticator or Aegis. A convincing fake sign-in page can capture them.",
  "magic-link": "A single-use sign-in link or code sent by email. Only as safe as the mailbox it goes to.",
  "recovery-codes": "Single-use backup codes for people who lost their other methods.",
  password: "For people who can't use a passkey yet, such as during a move from another identity provider. Passwords can be phished, guessed or reused from a breach elsewhere.",
  federated: "Sign-in through Google, Microsoft Entra, GitHub or another OpenID Connect provider. Only as strong as that provider's sign-in.",
  "access-pass": "A time-limited pass an administrator gives someone to set up a new passkey. It never signs anyone in by itself.",
  certificate: "A certificate from your organization's certificate authority, such as on a smart card. Your reverse proxy asks the browser for it and passes it to Halo.",
};

const HAVE: Record<MethodKind, string> = {
  passkey: "a passkey",
  "security-key": "a security key",
  totp: "an authenticator app",
  "magic-link": "",
  "recovery-codes": "recovery codes",
  password: "a password",
  federated: "a linked identity provider account",
  "access-pass": "a temporary access pass",
  certificate: "",
};

function people(n: number) {
  return pluralize(n, "active person", "active people");
}

function usage(m: MethodSetting) {
  if (m.method === "certificate") return "Matches each certificate to a person through the trusted certificate authorities below.";
  if (m.method === "magic-link") return `${people(m.users)} ${m.users === 1 ? "has" : "have"} no passkey, security key or authenticator app.`;
  return `${people(m.users)} ${m.users === 1 ? "has" : "have"} ${HAVE[m.method]}.`;
}

function warning(m: MethodSetting) {
  if (m.method === "magic-link") return `${pluralize(m.exclusive, "person relies", "people rely")} on magic links because nothing else they enrolled is turned on.`;
  return `${pluralize(m.exclusive, "person has", "people have")} no other passkey, security key or authenticator app that is turned on.`;
}

export function MethodSettings({ methods, settings, groups }: { methods: MethodSetting[]; settings: Settings; groups: Option[] }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [confirming, setConfirming] = useState<MethodSetting | null>(null);
  const magicLinks = methods.find((m) => m.method === "magic-link")?.enabled ?? false;

  function apply(m: MethodSetting, enabled: boolean) {
    const plural = METHOD_PLURAL[m.method];
    void run(m.method, () => api(`/methods/${m.method}`, { method: "PUT", body: { enabled } }), () => {
      setConfirming(null);
      toast({
        title: `${plural[0]!.toUpperCase()}${plural.slice(1)} turned ${enabled ? "on" : "off"}`,
        description: !enabled ? "Sign-ins that use them are blocked from now on." : m.method === "password" ? "Choose who can use them under Password rules." : "People can sign in with them again.",
      });
    }, "inline");
  }

  function change(m: MethodSetting, enabled: boolean) {
    if ((!enabled && m.exclusive > 0) || (enabled && m.method === "password")) setConfirming(m);
    else apply(m, enabled);
  }

  function consequence(m: MethodSetting) {
    const who = `${people(m.exclusive)} ${m.exclusive === 1 ? "has" : "have"}`;
    if (m.method === "magic-link") {
      return `${who} no passkey, security key or authenticator app that is turned on. They can't sign in until you turn magic links back on or send them a setup link to enroll a passkey.`;
    }
    const fallback = magicLinks
      ? "They'll have to sign in with a magic link until they enroll another method."
      : "They can't sign in until you turn this back on or send them a new setup link.";
    return `${who} no other passkey, security key or authenticator app that is turned on. ${fallback}`;
  }

  return (
    <>
      <Card>
        <ul>
          {methods.map((m) => {
            const meta = METHOD[m.method];
            const Icon = meta.icon;
            const id = `method-${m.method}`;
            return (
              <li key={m.method} className="flex flex-col gap-4 border-b border-border px-4 py-6 last:border-b-0 sm:flex-row sm:items-start sm:justify-between sm:gap-8 sm:px-6">
                <div className="flex min-w-0 gap-4">
                  <span className="grid size-10 shrink-0 place-items-center rounded-md border border-border bg-sunken text-fg-3">
                    <Icon aria-hidden="true" size={20} strokeWidth={1.75} />
                  </span>
                  <div className="flex min-w-0 flex-col gap-1">
                    <span className="flex flex-wrap items-center gap-2">
                      <span id={id} className="font-medium text-fg">
                        {meta.label}
                      </span>
                      {meta.phishingResistant ? <Badge tone="success">Phishing-resistant</Badge> : null}
                      {m.enabled ? null : <Badge tone="neutral">Off</Badge>}
                    </span>
                    <p className="text-body-sm text-fg-3">{DESCRIPTION[m.method]}</p>
                    <p className="tnum text-body-sm text-fg-2">{usage(m)}</p>
                    {confirming ? null : <ActionError message={errorFor(m.method)} />}
                    {m.enabled && m.exclusive > 0 ? (
                      <p className="flex items-start gap-2 text-body-sm text-warning">
                        <TriangleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0" />
                        <span className="tnum">{warning(m)}</span>
                      </p>
                    ) : null}
                  </div>
                </div>
                <label className="flex h-8 shrink-0 cursor-pointer items-center gap-2 pl-14 text-body-sm text-fg sm:pl-0">
                  {pending === m.method ? (
                    <Spinner />
                  ) : (
                    <Checkbox aria-labelledby={`${id} ${id}-state`} checked={m.enabled} disabled={pending !== null} onChange={(event) => change(m, event.target.checked)} />
                  )}
                  <span id={`${id}-state`}>Allowed</span>
                </label>
              </li>
            );
          })}
        </ul>
      </Card>
      <p className="text-body-sm text-fg-3">
        Changes apply from the next sign-in. Enrolled methods are kept when you turn one off, so turning it back on restores them. People who are already signed in stay signed in.
      </p>
      {magicLinks ? <SignInEmails settings={settings} /> : null}
      {methods.some((m) => m.method === "password" && m.enabled) ? <PasswordRules settings={settings} groups={groups} /> : null}
      {methods.some((m) => (m.method === "passkey" || m.method === "security-key") && m.enabled) ? <PasskeyRules settings={settings} groups={groups} /> : null}
      <Dialog
        open={confirming?.method === "password" && !confirming.enabled}
        onOpenChange={(open) => {
          if (open) return;
          setConfirming(null);
          clearError();
        }}
        error={confirming ? errorFor(confirming.method) : undefined}
        title="Turn on passwords?"
        description="Passkeys stay the recommended way to sign in. Use passwords only for people who can't use a passkey yet, such as during a move from another identity provider."
        footer={
          <>
            <Button variant="secondary" onClick={() => setConfirming(null)}>
              Cancel
            </Button>
            <Button variant="primary" loading={pending === "password"} onClick={() => confirming && apply(confirming, true)}>
              Turn on passwords
            </Button>
          </>
        }
      >
        <SecretWarning>
          Passwords can be phished by a fake sign-in page, guessed, or reused from a breach at another site. Passkeys resist all three. After you turn passwords on, limit them to the groups that need them under Password rules.
        </SecretWarning>
      </Dialog>
      <Dialog
        open={confirming !== null && confirming.enabled}
        onOpenChange={(open) => {
          if (open) return;
          setConfirming(null);
          clearError();
        }}
        error={confirming ? errorFor(confirming.method) : undefined}
        title={confirming ? `Turn off ${METHOD_PLURAL[confirming.method]}?` : "Turn off this method?"}
        description={confirming ? consequence(confirming) : undefined}
        footer={
          <>
            <Button variant="secondary" onClick={() => setConfirming(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={confirming !== null && pending === confirming.method} onClick={() => confirming && apply(confirming, false)}>
              Turn off {confirming ? METHOD_PLURAL[confirming.method] : ""}
            </Button>
          </>
        }
      />
    </>
  );
}

function SignInEmails({ settings }: { settings: Settings }) {
  const { saving, failure, save } = useSaveSettings();
  const [contents, setContents] = useState(settings.emailSignIn);

  return (
    <Card>
      <CardHeader
        title="Sign-in emails"
        description="What people get when they ask for an email on the sign-in page. A code works when they read the email on another device, and a mail scanner that opens the link doesn't use up the code."
      />
      <CardBody>
        <form
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            void save({ emailSignIn: contents }, "Sign-in emails saved", "Emails sent from now on follow it within 30 seconds.");
          }}
        >
          <Field label="Each email contains" hint="Links and codes work once and expire after 10 minutes. A code has 5 attempts, and wrong codes count toward the lockout.">
            {(props) => (
              <Select {...props} value={contents} onChange={(event) => setContents(event.target.value as Settings["emailSignIn"])}>
                <option value="link">A sign-in link</option>
                <option value="code">A 6-digit code to enter on the sign-in page</option>
                <option value="both">A sign-in link and a 6-digit code</option>
              </Select>
            )}
          </Field>
          <div className="flex flex-wrap items-center gap-4">
            <Button type="submit" variant="primary" loading={saving} disabled={contents === settings.emailSignIn}>
              Save sign-in emails
            </Button>
            <ActionError message={failure} />
          </div>
        </form>
      </CardBody>
    </Card>
  );
}

function PasswordRules({ settings, groups }: { settings: Settings; groups: Option[] }) {
  const { saving, save } = useSaveSettings();
  const [limited, setLimited] = useState(settings.passwordGroupIds.length > 0);
  const [groupIds, setGroupIds] = useState(() => settings.passwordGroupIds.filter((id) => groups.some((g) => g.id === id)));
  const [secondFactor, setSecondFactor] = useState(settings.passwordSecondFactor);
  const [breachCheck, setBreachCheck] = useState(settings.passwordBreachCheck);
  const [attempted, setAttempted] = useState(false);
  const chosen = limited ? groupIds : [];
  const problem = limited && groupIds.length === 0 ? "Choose at least one group, or let everyone use a password." : undefined;
  const dirty =
    secondFactor !== settings.passwordSecondFactor ||
    breachCheck !== settings.passwordBreachCheck ||
    chosen.length !== settings.passwordGroupIds.length ||
    chosen.some((id) => !settings.passwordGroupIds.includes(id));

  function submit() {
    setAttempted(true);
    if (problem) return;
    void save({ passwordGroupIds: chosen, passwordSecondFactor: secondFactor, passwordBreachCheck: breachCheck }, "Password rules saved", "Sign-ins follow them within 30 seconds.");
  }

  return (
    <Card>
      <CardHeader title="Password rules" description="Who can sign in with a password, and what else they need. Halo stores only Argon2id hashes of passwords and never shows them to anyone." />
      <CardBody>
        <form
          noValidate
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label="Who can use a password">
            {(props) => (
              <Select {...props} value={limited ? "groups" : "everyone"} onChange={(event) => setLimited(event.target.value === "groups")}>
                <option value="everyone">Everyone in the organization</option>
                <option value="groups">Members of selected groups</option>
              </Select>
            )}
          </Field>
          {limited ? (
            <div className="flex flex-col gap-2">
              <CheckList
                legend="Groups"
                options={groups}
                selected={groupIds}
                onToggle={(id, on) => setGroupIds((current) => (on ? [...current, id] : current.filter((v) => v !== id)))}
                empty="There are no groups yet."
              />
              <FieldError>{attempted ? problem : undefined}</FieldError>
            </div>
          ) : null}
          <label className="flex cursor-pointer items-start gap-3">
            <Checkbox className="mt-0.5" checked={secondFactor} onChange={(event) => setSecondFactor(event.target.checked)} />
            <span className="flex flex-col">
              <span className="text-body-sm text-fg">Also require a code from an authenticator app</span>
              <span className="text-caption text-fg-3">Recommended. People without an authenticator app can't sign in with a password, but can still use their other methods.</span>
            </span>
          </label>
          <label className="flex cursor-pointer items-start gap-3">
            <Checkbox className="mt-0.5" checked={breachCheck} onChange={(event) => setBreachCheck(event.target.checked)} />
            <span className="flex flex-col">
              <span className="text-body-sm text-fg">Refuse passwords found in data breaches</span>
              <span className="text-caption text-fg-3">
                Checks new passwords with Have I Been Pwned, sending only the first 5 characters of their SHA-1 hash. If the service can&apos;t be reached, Halo accepts the password and logs a warning. Turn this off when Halo has no internet access.
              </span>
            </span>
          </label>
          <div>
            <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
              Save password rules
            </Button>
          </div>
        </form>
      </CardBody>
    </Card>
  );
}

type PasskeyRule = Settings["authenticatorRules"][number];
type PasskeyRuleRow = Omit<PasskeyRule, "aaguids" | "minimum"> & { aaguids: string; minimum: string };

const AAGUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

function aaguidList(text: string) {
  return [...new Set(text.toLowerCase().split(/[\s,]+/).filter(Boolean))];
}

function ruleProblem(row: PasskeyRuleRow) {
  const minimum = Number(row.minimum);
  const invalid = row.attestation ? aaguidList(row.aaguids).find((a) => !AAGUID.test(a)) : undefined;
  if (!row.groupId) return "Choose a group, or remove these rules.";
  if (row.minimum.trim() === "" || !Number.isInteger(minimum) || minimum < 0 || minimum > 10) return "Enter a whole number of keys between 0 and 10.";
  if (invalid) return `“${invalid}” is not an AAGUID. Enter each model like cb69481e-8ff7-4039-93ec-0a2729a154a8.`;
  if (!row.deviceBound && !row.attestation && minimum === 0) return "Choose at least one rule for this group, or remove it.";
  return undefined;
}

function PasskeyRules({ settings, groups }: { settings: Settings; groups: Option[] }) {
  const { saving, failure, save } = useSaveSettings();
  const saved = settings.authenticatorRules.filter((rule) => groups.some((g) => g.id === rule.groupId));
  const [rows, setRows] = useState<PasskeyRuleRow[]>(() => saved.map((rule) => ({ ...rule, aaguids: rule.aaguids.join("\n"), minimum: String(rule.minimum) })));
  const [attempted, setAttempted] = useState(false);
  const rules: PasskeyRule[] = rows.map((row) => ({
    groupId: row.groupId,
    deviceBound: row.deviceBound,
    attestation: row.attestation,
    aaguids: row.attestation ? aaguidList(row.aaguids) : [],
    minimum: Number(row.minimum),
  }));
  const dirty = JSON.stringify(rules) !== JSON.stringify(saved);
  const groupName = (id: string) => groups.find((g) => g.id === id)?.name ?? "this group";

  function update(index: number, change: Partial<PasskeyRuleRow>) {
    setRows((current) => current.map((row, i) => (i === index ? { ...row, ...change } : row)));
  }

  function submit() {
    setAttempted(true);
    if (rows.some((row) => ruleProblem(row))) return;
    void save({ authenticatorRules: rules }, "Passkey rules saved", "New passkeys and security keys follow them within 30 seconds. Keys people already have keep working.");
  }

  return (
    <Card>
      <CardHeader
        title="Passkey rules"
        description="Limit which passkeys and security keys members of a group can add. Someone in several of these groups must meet the rules of each. Keys added before a rule keep working."
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
          {rows.length === 0 ? <p className="text-body-sm text-fg-3">No group has passkey rules, so everyone can add any passkey or security key.</p> : null}
          {rows.map((row, index) => (
            <fieldset key={index} className="flex flex-col gap-4 rounded-lg border border-border p-4">
              <legend className="sr-only">Rules for {groupName(row.groupId)}</legend>
              <div className="flex items-center gap-3">
                <Select aria-label={`Group ${index + 1}`} value={row.groupId} onChange={(event) => update(index, { groupId: event.target.value })}>
                  <option value="">Choose a group</option>
                  {groups
                    .filter((g) => g.id === row.groupId || !rows.some((other) => other.groupId === g.id))
                    .map((g) => (
                      <option key={g.id} value={g.id}>
                        {g.name}
                      </option>
                    ))}
                </Select>
                <Button size="icon-sm" variant="quiet" aria-label={`Remove the rules for ${groupName(row.groupId)}`} onClick={() => setRows((current) => current.filter((_, i) => i !== index))}>
                  <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
              </div>
              <label className="flex cursor-pointer items-start gap-3">
                <Checkbox className="mt-0.5" checked={row.deviceBound} onChange={(event) => update(index, { deviceBound: event.target.checked })} />
                <span className="flex flex-col">
                  <span className="text-body-sm text-fg">Only keys that stay on one device</span>
                  <span className="text-caption text-fg-3">Refuses passkeys that sync between devices, such as those in iCloud Keychain or Google Password Manager. Security keys qualify.</span>
                </span>
              </label>
              <label className="flex cursor-pointer items-start gap-3">
                <Checkbox className="mt-0.5" checked={row.attestation} onChange={(event) => update(index, { attestation: event.target.checked })} />
                <span className="flex flex-col">
                  <span className="text-body-sm text-fg">Only certified models</span>
                  <span className="text-caption text-fg-3">
                    Each new key must prove its make and model, checked against the FIDO Metadata Service, so counterfeit keys and keys that hide their model are refused. Halo downloads the service&apos;s list once a day, so it needs internet access.
                  </span>
                </span>
              </label>
              {row.attestation ? (
                <Field label="Approved models" hint="One AAGUID per line, from the key maker or the FIDO Metadata Service. Leave empty to accept any certified model.">
                  {(props) => <Textarea {...props} mono rows={3} value={row.aaguids} onChange={(event) => update(index, { aaguids: event.target.value })} />}
                </Field>
              ) : null}
              <Field label="Keys each member keeps" hint="Members can't remove a key that would leave them with fewer that meet these rules. 0 for no minimum.">
                {(props) => (
                  <div className="flex items-center gap-3">
                    <div className="w-24">
                      <Input {...props} type="number" inputMode="numeric" min={0} max={10} step={1} className="tnum" value={row.minimum} onChange={(event) => update(index, { minimum: event.target.value })} />
                    </div>
                    <span className="text-body-sm text-fg-3">keys</span>
                  </div>
                )}
              </Field>
              <FieldError>{attempted ? ruleProblem(row) : undefined}</FieldError>
            </fieldset>
          ))}
          <div>
            <Button
              size="sm"
              variant="secondary"
              disabled={rows.length >= groups.length}
              onClick={() => setRows((current) => [...current, { groupId: "", deviceBound: true, attestation: false, aaguids: "", minimum: "0" }])}
            >
              <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
              Add group rules
            </Button>
          </div>
          <div className="flex flex-wrap items-center gap-4">
            <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
              Save passkey rules
            </Button>
            <ActionError message={failure} />
          </div>
        </form>
      </CardBody>
    </Card>
  );
}
