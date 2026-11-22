"use client";

import Link from "next/link";
import { TriangleAlert, X } from "lucide-react";
import { useState, type ReactNode } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Badge } from "@/components/ui/badge";
import { Button, buttonClasses } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { HoldToConfirm } from "@/components/ui/hold-to-confirm";
import { Checkbox, Field, Input, Select, Textarea } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { cn } from "@/lib/cn";
import { useMessages } from "@/lib/i18n/client";
import { policyMessages } from "@/lib/i18n/console/policy";
import type { AccessPolicy, ComplianceCondition, DeviceCondition, NetworkCondition, PolicyConditions, PolicyEffect, PolicyMode } from "@/lib/policy-types";
import { HALO_APP, POLICY_METHODS, RISK_LEVELS, describePolicy, policyState, type Names } from "./shared";

export type Option = { id: string; name: string; detail?: string };

type ListKey = "groupIds" | "userIds" | "excludeGroupIds" | "excludeUserIds" | "appIds" | "zoneIds";

const BLANK: PolicyConditions = {
  allUsers: true,
  groupIds: [],
  userIds: [],
  excludeGroupIds: [],
  excludeUserIds: [],
  allApps: true,
  appIds: [],
  network: "any",
  zoneIds: [],
  device: "any",
  compliance: "any",
  risk: [],
  methods: [],
};

function nameMap(options: Option[]) {
  return Object.fromEntries(options.map((o) => [o.id, o.name]));
}

export function FieldError({ children }: { children?: string }) {
  return children ? <p className="text-caption text-danger">{children}</p> : null;
}

export function CheckList({
  legend,
  options,
  selected,
  onToggle,
  empty,
  scroll = true,
}: {
  legend: string;
  options: Option[];
  selected: string[];
  onToggle: (id: string, on: boolean) => void;
  empty: ReactNode;
  scroll?: boolean;
}) {
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="mb-2 text-label text-fg">{legend}</legend>
      {options.length ? (
        <div className={cn("flex flex-col gap-3 rounded-md border border-border bg-sunken p-4", scroll && "max-h-64 overflow-y-auto")}>
          {options.map((o) => (
            <label key={o.id} className="flex cursor-pointer items-start gap-3">
              <Checkbox className="mt-0.5" checked={selected.includes(o.id)} onChange={(event) => onToggle(o.id, event.target.checked)} />
              <span className="flex min-w-0 flex-col">
                <span className="text-body-sm text-fg">{o.name}</span>
                {o.detail ? <span className="text-caption text-fg-3">{o.detail}</span> : null}
              </span>
            </label>
          ))}
        </div>
      ) : (
        <p className="text-body-sm text-fg-3">{empty}</p>
      )}
    </fieldset>
  );
}

function ReferenceField({
  label,
  hint,
  placeholder,
  empty,
  tab,
  link,
  options,
  value,
  onChange,
  error,
}: {
  label: string;
  hint: string;
  placeholder: string;
  empty: string;
  tab: string;
  link: string;
  options: Option[];
  value: string;
  onChange: (id: string) => void;
  error?: string;
}) {
  if (!options.length) {
    return (
      <div className="flex flex-col gap-2">
        <p className="text-body-sm text-fg-3">
          {empty}{" "}
          <Link href={`/admin/policies?tab=${tab}`} className="text-link underline underline-offset-3 hover:text-ember">
            {link}
          </Link>
          .
        </p>
        <FieldError>{error}</FieldError>
      </div>
    );
  }
  return (
    <Field label={label} error={error} hint={hint}>
      {(props) => (
        <Select {...props} value={value} onChange={(event) => onChange(event.target.value)}>
          <option value="">{placeholder}</option>
          {options.map((o) => (
            <option key={o.id} value={o.id}>
              {o.name} · {o.detail}
            </option>
          ))}
        </Select>
      )}
    </Field>
  );
}

function PeoplePicker({ label, users, selected, onChange }: { label: string; users: Option[]; selected: string[]; onChange: (ids: string[]) => void }) {
  const t = useMessages(policyMessages).form;
  const names = nameMap(users);
  return (
    <div className="flex flex-col gap-3">
      <Field label={label}>
        {(props) => (
          <Select
            {...props}
            value=""
            onChange={(event) => {
              if (event.target.value) onChange([...selected, event.target.value]);
            }}
          >
            <option value="">{t.addPerson}</option>
            {users
              .filter((u) => !selected.includes(u.id))
              .map((u) => (
                <option key={u.id} value={u.id}>
                  {u.detail ? `${u.name} · ${u.detail}` : u.name}
                </option>
              ))}
          </Select>
        )}
      </Field>
      {selected.length ? (
        <ul className="flex flex-wrap gap-2" aria-label={label}>
          {selected.map((id) => (
            <li key={id} className="inline-flex h-8 items-center gap-1 rounded-md border border-border-strong bg-sunken pr-1 pl-3 text-body-sm text-fg">
              {names[id] ?? t.deletedUser}
              <button
                type="button"
                aria-label={names[id] ? t.removePerson(names[id]) : t.removeDeletedUser}
                onClick={() => onChange(selected.filter((v) => v !== id))}
                className="grid size-6 place-items-center rounded-sm text-fg-3 transition-colors duration-fast hover:bg-hover hover:text-fg"
              >
                <X aria-hidden="true" size={16} strokeWidth={1.75} />
              </button>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

export function PolicyForm({
  policy,
  groups,
  users,
  applications,
  zones,
  terms,
  contexts,
}: {
  policy?: AccessPolicy;
  groups: Option[];
  users: Option[];
  applications: Option[];
  zones: Option[];
  terms: Option[];
  contexts: Option[];
}) {
  const copy = useMessages(policyMessages);
  const t = copy.form;
  const { pending, run, errorFor, toast, router } = useMutation();
  const [name, setName] = useState(policy?.name ?? "");
  const [description, setDescription] = useState(policy?.description ?? "");
  const [effect, setEffect] = useState<PolicyEffect>(policy?.effect ?? "block");
  const [termsId, setTermsId] = useState(policy?.termsId ?? "");
  const [contextId, setContextId] = useState(policy?.contextId ?? "");
  const [mode, setMode] = useState<PolicyMode>(policy?.mode ?? "report");
  const [enabled, setEnabled] = useState(policy?.enabled ?? true);
  const [c, setC] = useState<PolicyConditions>(policy?.conditions ?? BLANK);
  const [attempted, setAttempted] = useState(false);

  const apps: Option[] = [{ id: HALO_APP, name: "Halo", detail: t.haloDetail }, ...applications];
  const names: Names = { groups: nameMap(groups), users: nameMap(users), apps: nameMap(applications), zones: nameMap(zones), terms: nameMap(terms), contexts: nameMap(contexts) };
  const state = policyState({ enabled, mode }, copy);
  const problems = {
    name: name.trim() ? undefined : copy.enterName(t.nameExample),
    users: c.allUsers || c.groupIds.length + c.userIds.length ? undefined : t.usersRequired,
    apps: c.allApps || c.appIds.length ? undefined : t.appsRequired,
    zones: c.network === "any" || c.zoneIds.length ? undefined : t.zonesRequired,
    terms: effect !== "require-terms" || termsId ? undefined : t.termsRequired,
    context: effect !== "require-context" || contextId ? undefined : t.contextRequired,
  };
  const invalid = Object.values(problems).some(Boolean);
  const errors: Partial<typeof problems> = attempted ? problems : {};
  const lockout = enabled && mode === "enforce" && effect !== "allow" && effect !== "require-terms" && c.allUsers && c.excludeGroupIds.length + c.excludeUserIds.length === 0;

  function set<K extends keyof PolicyConditions>(key: K, value: PolicyConditions[K]) {
    setC((current) => ({ ...current, [key]: value }));
  }

  function toggle(key: ListKey, id: string, on: boolean) {
    setC((current) => ({ ...current, [key]: on ? [...current[key], id] : current[key].filter((v) => v !== id) }));
  }

  function save() {
    setAttempted(true);
    if (invalid) return;
    const body = { name: name.trim(), description: description.trim(), enabled, mode, effect, termsId: effect === "require-terms" ? termsId : null, contextId: effect === "require-context" ? contextId : null, conditions: c };
    void run(
      "save",
      () => (policy ? api(`/policies/${policy.id}`, { method: "PUT", body }) : api("/policies", { body })),
      () => {
        toast({
          title: policy ? copy.saved(name.trim()) : t.created(name.trim()),
          description: !enabled ? t.staysOff : mode === "report" ? copy.labels.mode.report : t.appliesNext,
        });
        router.push("/admin/policies");
      },
      "inline",
    );
  }

  async function remove() {
    if (!policy) return false;
    return run("delete", () => api(`/policies/${policy.id}`, { method: "DELETE" }), () => {
      toast({ title: copy.deleted(policy.name), description: copy.list.deletedDescription });
      router.push("/admin/policies");
    }, "inline");
  }

  return (
    <form
      className="grid grid-cols-1 gap-8 xl:grid-cols-12"
      onSubmit={(event) => {
        event.preventDefault();
        save();
      }}
    >
      <div className="flex min-w-0 flex-col gap-6 xl:col-span-8">
        <Card>
          <CardHeader title={t.details} />
          <CardBody className="flex flex-col gap-6">
            <Field label={copy.name} error={errors.name}>
              {(props) => <Input {...props} autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder={t.nameExample} />}
            </Field>
            <Field label={t.description} hint={t.descriptionHint}>
              {(props) => <Textarea {...props} rows={2} value={description} onChange={(event) => setDescription(event.target.value)} />}
            </Field>
          </CardBody>
        </Card>

        <Card>
          <CardHeader title={t.people} description={t.peopleDescription} />
          <CardBody className="flex flex-col gap-6">
            <Field label={t.appliesTo}>
              {(props) => (
                <Select {...props} value={c.allUsers ? "all" : "selected"} onChange={(event) => set("allUsers", event.target.value === "all")}>
                  <option value="all">{t.allUsers}</option>
                  <option value="selected">{t.selectedPeople}</option>
                </Select>
              )}
            </Field>
            {c.allUsers ? null : (
              <div className="flex flex-col gap-6">
                <CheckList legend={copy.groups} options={groups} selected={c.groupIds} onToggle={(id, on) => toggle("groupIds", id, on)} empty={copy.noGroups} />
                <PeoplePicker label={t.people} users={users} selected={c.userIds} onChange={(ids) => set("userIds", ids)} />
                <FieldError>{errors.users}</FieldError>
              </div>
            )}
            <div className="flex flex-col gap-6 border-t border-border pt-6">
              <CheckList
                legend={t.excludeGroups}
                options={groups}
                selected={c.excludeGroupIds}
                onToggle={(id, on) => toggle("excludeGroupIds", id, on)}
                empty={copy.noGroups}
              />
              <PeoplePicker label={t.excludePeople} users={users} selected={c.excludeUserIds} onChange={(ids) => set("excludeUserIds", ids)} />
              <p className="text-caption text-fg-3">{t.breakGlass}</p>
            </div>
          </CardBody>
        </Card>

        <Card>
          <CardHeader title={t.applications} description={t.applicationsDescription} />
          <CardBody className="flex flex-col gap-6">
            <Field label={t.covers}>
              {(props) => (
                <Select {...props} value={c.allApps ? "all" : "selected"} onChange={(event) => set("allApps", event.target.value === "all")}>
                  <option value="all">{t.allApps}</option>
                  <option value="selected">{t.selectedApps}</option>
                </Select>
              )}
            </Field>
            {c.allApps ? null : (
              <div className="flex flex-col gap-2">
                <CheckList legend={t.applications} options={apps} selected={c.appIds} onToggle={(id, on) => toggle("appIds", id, on)} empty={t.noApps} />
                <FieldError>{errors.apps}</FieldError>
              </div>
            )}
          </CardBody>
        </Card>

        <Card>
          <CardHeader title={t.conditions} description={t.conditionsDescription} />
          <CardBody className="flex flex-col gap-6">
            <Field label={t.network}>
              {(props) => (
                <Select {...props} value={c.network} onChange={(event) => set("network", event.target.value as NetworkCondition)}>
                  {(Object.keys(copy.labels.network) as NetworkCondition[]).map((value) => (
                    <option key={value} value={value}>
                      {copy.labels.network[value]}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            {c.network === "any" ? null : (
              <div className="flex flex-col gap-2">
                <CheckList
                  legend={t.networks}
                  options={zones}
                  selected={c.zoneIds}
                  onToggle={(id, on) => toggle("zoneIds", id, on)}
                  empty={
                    <>
                      {t.noNetworks}{" "}
                      <Link href="/admin/policies?tab=networks" className="text-link underline underline-offset-3 hover:text-ember">
                        {t.addNetwork}
                      </Link>
                      .
                    </>
                  }
                />
                <FieldError>{errors.zones}</FieldError>
              </div>
            )}
            <Field label={t.device} hint={t.deviceHint}>
              {(props) => (
                <Select {...props} value={c.device} onChange={(event) => set("device", event.target.value as DeviceCondition)}>
                  {(Object.keys(copy.labels.device) as DeviceCondition[]).map((value) => (
                    <option key={value} value={value}>
                      {copy.labels.device[value]}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            <Field label={t.compliance} hint={t.complianceHint}>
              {(props) => (
                <Select {...props} value={c.compliance} onChange={(event) => set("compliance", event.target.value as ComplianceCondition)}>
                  {(Object.keys(copy.labels.complianceCondition) as ComplianceCondition[]).map((value) => (
                    <option key={value} value={value}>
                      {copy.labels.complianceCondition[value]}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            <fieldset className="flex flex-col gap-3">
              <legend className="mb-2 text-label text-fg">{t.risk}</legend>
              <div className="flex flex-wrap gap-x-6 gap-y-3">
                {RISK_LEVELS.map((level) => (
                  <label key={level} className="flex cursor-pointer items-center gap-2 text-body-sm text-fg">
                    <Checkbox
                      checked={c.risk.includes(level)}
                      onChange={(event) => set("risk", event.target.checked ? [...c.risk, level] : c.risk.filter((v) => v !== level))}
                    />
                    {copy.labels.risk[level]}
                  </label>
                ))}
              </div>
              <p className="text-caption text-fg-3">{t.riskHint}</p>
            </fieldset>
            <fieldset className="flex flex-col gap-3">
              <legend className="mb-2 text-label text-fg">{t.method}</legend>
              <div className="flex flex-wrap gap-x-6 gap-y-3">
                {POLICY_METHODS.map((kind) => (
                  <label key={kind} className="flex cursor-pointer items-center gap-2 text-body-sm text-fg">
                    <Checkbox
                      checked={c.methods.includes(kind)}
                      onChange={(event) => set("methods", event.target.checked ? [...c.methods, kind] : c.methods.filter((v) => v !== kind))}
                    />
                    {copy.labels.method[kind]}
                  </label>
                ))}
              </div>
              <p className="text-caption text-fg-3">{t.methodHint}</p>
            </fieldset>
          </CardBody>
        </Card>

        <Card>
          <CardHeader title={t.effect} description={t.effectDescription} />
          <CardBody className="flex flex-col gap-6">
            <Field label={t.effect} hint={t.effectHint}>
              {(props) => (
                <Select {...props} value={effect} onChange={(event) => setEffect(event.target.value as PolicyEffect)}>
                  {(Object.keys(copy.labels.effect) as PolicyEffect[]).map((value) => (
                    <option key={value} value={value}>
                      {copy.labels.effect[value]}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            {effect === "require-terms" ? (
              <ReferenceField
                label={t.terms}
                hint={t.termsHint}
                placeholder={t.chooseTerms}
                empty={t.noTerms}
                tab="terms"
                link={t.addTerms}
                options={terms}
                value={termsId}
                onChange={setTermsId}
                error={errors.terms}
              />
            ) : null}
            {effect === "require-context" ? (
              <ReferenceField
                label={t.context}
                hint={t.contextHint}
                placeholder={t.chooseContext}
                empty={t.noContexts}
                tab="contexts"
                link={t.addContext}
                options={contexts}
                value={contextId}
                onChange={setContextId}
                error={errors.context}
              />
            ) : null}
            <Field label={t.mode} hint={copy.labels.mode[mode]}>
              {(props) => (
                <Select {...props} value={mode} onChange={(event) => setMode(event.target.value as PolicyMode)}>
                  <option value="report">{copy.labels.state.report}</option>
                  <option value="enforce">{t.enforce}</option>
                </Select>
              )}
            </Field>
            <label className="flex cursor-pointer items-start gap-3">
              <Checkbox className="mt-0.5" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} />
              <span className="flex flex-col">
                <span className="text-body-sm text-fg">{t.enabled}</span>
                <span className="text-caption text-fg-3">{t.enabledHint}</span>
              </span>
            </label>
          </CardBody>
        </Card>
      </div>

      <aside className="xl:col-span-4">
        <div className="flex flex-col gap-6 xl:sticky xl:top-24">
          <Card>
            <CardHeader title={t.summary} actions={<Badge tone={state.tone} dot>{state.label}</Badge>} />
            <CardBody className="flex flex-col gap-4">
              <p className="text-body text-fg">{describePolicy({ effect, termsId, contextId, conditions: c }, names, copy)}</p>
              {lockout ? (
                <div role="note" className="flex gap-3 rounded-md border border-warning/20 bg-warning-container p-4">
                  <TriangleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0 text-warning" />
                  <p className="text-body-sm text-fg-2">{t.lockout}</p>
                </div>
              ) : null}
              <ActionError message={errorFor("save")} />
              <div className="flex flex-wrap gap-2 pt-2">
                <Button type="submit" variant="primary" loading={pending === "save"}>
                  {policy ? t.save : copy.page.create}
                </Button>
                <Link href="/admin/policies" className={buttonClasses("secondary")}>
                  {copy.cancel}
                </Link>
              </div>
              {attempted && invalid ? <p className="text-caption text-danger">{t.fixFields}</p> : null}
            </CardBody>
          </Card>
          {policy ? (
            <Card>
              <CardHeader title={copy.list.delete} description={copy.list.deleteDescription} />
              <CardBody className="flex flex-col items-start gap-3">
                <HoldToConfirm size="sm" doneLabel={t.deletedLabel} onConfirm={remove}>
                  {t.holdToDelete}
                </HoldToConfirm>
                <ActionError message={errorFor("delete")} />
              </CardBody>
            </Card>
          ) : null}
        </div>
      </aside>
    </form>
  );
}
