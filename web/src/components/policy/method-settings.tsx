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
import { useMessages } from "@/lib/i18n/client";
import { policyMessages } from "@/lib/i18n/console/policy";
import { METHOD } from "@/lib/labels";
import type { MethodSetting } from "@/lib/policy-types";
import type { Settings } from "@/lib/settings-types";
import { CheckList, FieldError, type Option } from "./policy-form";
import type { PolicyCopy } from "./shared";

function usage(m: MethodSetting, t: PolicyCopy["methods"]) {
  if (m.method === "certificate") return t.usageCertificate;
  if (m.method === "kerberos") return t.usageKerberos;
  if (m.method === "magic-link") return t.usageMagicLink(m.users);
  return t.usage(m.users, t.have[m.method]);
}

function warning(m: MethodSetting, t: PolicyCopy["methods"]) {
  if (m.method === "magic-link") return t.warningMagicLink(m.exclusive);
  return t.warning(m.exclusive);
}

export function MethodSettings({ methods, settings, groups }: { methods: MethodSetting[]; settings: Settings; groups: Option[] }) {
  const copy = useMessages(policyMessages);
  const t = copy.methods;
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [confirming, setConfirming] = useState<MethodSetting | null>(null);
  const magicLinks = methods.find((m) => m.method === "magic-link")?.enabled ?? false;

  function apply(m: MethodSetting, enabled: boolean) {
    void run(m.method, () => api(`/methods/${m.method}`, { method: "PUT", body: { enabled } }), () => {
      setConfirming(null);
      toast({
        title: t.turned(copy.labels.method[m.method], copy.labels.methodPlural[m.method], enabled),
        description: !enabled ? t.turnedOff : m.method === "password" ? t.passwordsOn : t.turnedOn,
      });
    }, "inline");
  }

  function change(m: MethodSetting, enabled: boolean) {
    if ((!enabled && m.exclusive > 0) || (enabled && m.method === "password")) setConfirming(m);
    else apply(m, enabled);
  }

  function consequence(m: MethodSetting) {
    if (m.method === "magic-link") return t.consequenceMagicLink(m.exclusive);
    return t.consequence(m.exclusive, magicLinks);
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
                        {copy.labels.method[m.method]}
                      </span>
                      {meta.phishingResistant ? <Badge tone="success">{t.phishingResistant}</Badge> : null}
                      {m.enabled ? null : <Badge tone="neutral">{t.off}</Badge>}
                    </span>
                    <p className="text-body-sm text-fg-3">{t.descriptions[m.method]}</p>
                    <p className="tnum text-body-sm text-fg-2">{usage(m, t)}</p>
                    {confirming ? null : <ActionError message={errorFor(m.method)} />}
                    {m.enabled && m.exclusive > 0 ? (
                      <p className="flex items-start gap-2 text-body-sm text-warning">
                        <TriangleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0" />
                        <span className="tnum">{warning(m, t)}</span>
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
                  <span id={`${id}-state`}>{t.allowed}</span>
                </label>
              </li>
            );
          })}
        </ul>
      </Card>
      <p className="text-body-sm text-fg-3">
        {t.footnote}
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
        title={t.passwordsTitle}
        description={t.passwordsDescription}
        footer={
          <>
            <Button variant="secondary" onClick={() => setConfirming(null)}>
              {copy.cancel}
            </Button>
            <Button variant="primary" loading={pending === "password"} onClick={() => confirming && apply(confirming, true)}>
              {t.passwordsConfirm}
            </Button>
          </>
        }
      >
        <SecretWarning>
          {t.passwordsWarning}
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
        title={confirming ? t.turnOffTitle(copy.labels.methodPlural[confirming.method]) : t.turnOffFallback}
        description={confirming ? consequence(confirming) : undefined}
        footer={
          <>
            <Button variant="secondary" onClick={() => setConfirming(null)}>
              {copy.cancel}
            </Button>
            <Button variant="destructive" loading={confirming !== null && pending === confirming.method} onClick={() => confirming && apply(confirming, false)}>
              {t.turnOff(confirming ? copy.labels.methodPlural[confirming.method] : "")}
            </Button>
          </>
        }
      />
    </>
  );
}

function SignInEmails({ settings }: { settings: Settings }) {
  const t = useMessages(policyMessages).methods;
  const { saving, failure, save } = useSaveSettings();
  const [contents, setContents] = useState(settings.emailSignIn);

  return (
    <Card>
      <CardHeader
        title={t.emails}
        description={t.emailsDescription}
      />
      <CardBody>
        <form
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            void save({ emailSignIn: contents }, t.emailsSaved, t.emailsSavedDescription);
          }}
        >
          <Field label={t.emailContains} hint={t.emailContainsHint}>
            {(props) => (
              <Select {...props} value={contents} onChange={(event) => setContents(event.target.value as Settings["emailSignIn"])}>
                <option value="link">{t.emailLink}</option>
                <option value="code">{t.emailCode}</option>
                <option value="both">{t.emailBoth}</option>
              </Select>
            )}
          </Field>
          <div className="flex flex-wrap items-center gap-4">
            <Button type="submit" variant="primary" loading={saving} disabled={contents === settings.emailSignIn}>
              {t.saveEmails}
            </Button>
            <ActionError message={failure} />
          </div>
        </form>
      </CardBody>
    </Card>
  );
}

function PasswordRules({ settings, groups }: { settings: Settings; groups: Option[] }) {
  const copy = useMessages(policyMessages);
  const t = copy.methods;
  const { saving, save } = useSaveSettings();
  const [limited, setLimited] = useState(settings.passwordGroupIds.length > 0);
  const [groupIds, setGroupIds] = useState(() => settings.passwordGroupIds.filter((id) => groups.some((g) => g.id === id)));
  const [secondFactor, setSecondFactor] = useState(settings.passwordSecondFactor);
  const [breachCheck, setBreachCheck] = useState(settings.passwordBreachCheck);
  const [attempted, setAttempted] = useState(false);
  const chosen = limited ? groupIds : [];
  const problem = limited && groupIds.length === 0 ? t.passwordGroupsRequired : undefined;
  const dirty =
    secondFactor !== settings.passwordSecondFactor ||
    breachCheck !== settings.passwordBreachCheck ||
    chosen.length !== settings.passwordGroupIds.length ||
    chosen.some((id) => !settings.passwordGroupIds.includes(id));

  function submit() {
    setAttempted(true);
    if (problem) return;
    void save({ passwordGroupIds: chosen, passwordSecondFactor: secondFactor, passwordBreachCheck: breachCheck }, t.passwordRulesSaved, t.passwordRulesSavedDescription);
  }

  return (
    <Card>
      <CardHeader title={t.passwordRules} description={t.passwordRulesDescription} />
      <CardBody>
        <form
          noValidate
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <Field label={t.whoPassword}>
            {(props) => (
              <Select {...props} value={limited ? "groups" : "everyone"} onChange={(event) => setLimited(event.target.value === "groups")}>
                <option value="everyone">{t.everyone}</option>
                <option value="groups">{t.selectedGroups}</option>
              </Select>
            )}
          </Field>
          {limited ? (
            <div className="flex flex-col gap-2">
              <CheckList
                legend={copy.groups}
                options={groups}
                selected={groupIds}
                onToggle={(id, on) => setGroupIds((current) => (on ? [...current, id] : current.filter((v) => v !== id)))}
                empty={copy.noGroups}
              />
              <FieldError>{attempted ? problem : undefined}</FieldError>
            </div>
          ) : null}
          <label className="flex cursor-pointer items-start gap-3">
            <Checkbox className="mt-0.5" checked={secondFactor} onChange={(event) => setSecondFactor(event.target.checked)} />
            <span className="flex flex-col">
              <span className="text-body-sm text-fg">{t.secondFactor}</span>
              <span className="text-caption text-fg-3">{t.secondFactorHint}</span>
            </span>
          </label>
          <label className="flex cursor-pointer items-start gap-3">
            <Checkbox className="mt-0.5" checked={breachCheck} onChange={(event) => setBreachCheck(event.target.checked)} />
            <span className="flex flex-col">
              <span className="text-body-sm text-fg">{t.breachCheck}</span>
              <span className="text-caption text-fg-3">{t.breachCheckHint}</span>
            </span>
          </label>
          <div>
            <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
              {t.savePasswordRules}
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

function ruleProblem(row: PasskeyRuleRow, t: PolicyCopy["methods"]) {
  const minimum = Number(row.minimum);
  const invalid = row.attestation ? aaguidList(row.aaguids).find((a) => !AAGUID.test(a)) : undefined;
  if (!row.groupId) return t.ruleGroup;
  if (row.minimum.trim() === "" || !Number.isInteger(minimum) || minimum < 0 || minimum > 10) return t.ruleMinimum;
  if (invalid) return t.ruleAaguid(invalid);
  if (!row.deviceBound && !row.attestation && minimum === 0) return t.ruleEmpty;
  return undefined;
}

function PasskeyRules({ settings, groups }: { settings: Settings; groups: Option[] }) {
  const t = useMessages(policyMessages).methods;
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
  const groupName = (id: string) => groups.find((g) => g.id === id)?.name ?? t.thisGroup;

  function update(index: number, change: Partial<PasskeyRuleRow>) {
    setRows((current) => current.map((row, i) => (i === index ? { ...row, ...change } : row)));
  }

  function submit() {
    setAttempted(true);
    if (rows.some((row) => ruleProblem(row, t))) return;
    void save({ authenticatorRules: rules }, t.passkeyRulesSaved, t.passkeyRulesSavedDescription);
  }

  return (
    <Card>
      <CardHeader
        title={t.passkeyRules}
        description={t.passkeyRulesDescription}
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
          {rows.length === 0 ? <p className="text-body-sm text-fg-3">{t.noPasskeyRules}</p> : null}
          {rows.map((row, index) => (
            <fieldset key={index} className="flex flex-col gap-4 rounded-lg border border-border p-4">
              <legend className="sr-only">{t.rulesFor(groupName(row.groupId))}</legend>
              <div className="flex items-center gap-3">
                <Select aria-label={t.groupNumber(index + 1)} value={row.groupId} onChange={(event) => update(index, { groupId: event.target.value })}>
                  <option value="">{t.chooseGroup}</option>
                  {groups
                    .filter((g) => g.id === row.groupId || !rows.some((other) => other.groupId === g.id))
                    .map((g) => (
                      <option key={g.id} value={g.id}>
                        {g.name}
                      </option>
                    ))}
                </Select>
                <Button size="icon-sm" variant="quiet" aria-label={t.removeRules(groupName(row.groupId))} onClick={() => setRows((current) => current.filter((_, i) => i !== index))}>
                  <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
              </div>
              <label className="flex cursor-pointer items-start gap-3">
                <Checkbox className="mt-0.5" checked={row.deviceBound} onChange={(event) => update(index, { deviceBound: event.target.checked })} />
                <span className="flex flex-col">
                  <span className="text-body-sm text-fg">{t.deviceBound}</span>
                  <span className="text-caption text-fg-3">{t.deviceBoundHint}</span>
                </span>
              </label>
              <label className="flex cursor-pointer items-start gap-3">
                <Checkbox className="mt-0.5" checked={row.attestation} onChange={(event) => update(index, { attestation: event.target.checked })} />
                <span className="flex flex-col">
                  <span className="text-body-sm text-fg">{t.attestation}</span>
                  <span className="text-caption text-fg-3">{t.attestationHint}</span>
                </span>
              </label>
              {row.attestation ? (
                <Field label={t.models} hint={t.modelsHint}>
                  {(props) => <Textarea {...props} mono rows={3} value={row.aaguids} onChange={(event) => update(index, { aaguids: event.target.value })} />}
                </Field>
              ) : null}
              <Field label={t.minimum} hint={t.minimumHint}>
                {(props) => (
                  <div className="flex items-center gap-3">
                    <div className="w-24">
                      <Input {...props} type="number" inputMode="numeric" min={0} max={10} step={1} className="tnum" value={row.minimum} onChange={(event) => update(index, { minimum: event.target.value })} />
                    </div>
                    <span className="text-body-sm text-fg-3">{t.keys}</span>
                  </div>
                )}
              </Field>
              <FieldError>{attempted ? ruleProblem(row, t) : undefined}</FieldError>
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
              {t.addGroupRules}
            </Button>
          </div>
          <div className="flex flex-wrap items-center gap-4">
            <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
              {t.savePasskeyRules}
            </Button>
            <ActionError message={failure} />
          </div>
        </form>
      </CardBody>
    </Card>
  );
}
