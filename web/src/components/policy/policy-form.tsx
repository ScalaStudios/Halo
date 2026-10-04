"use client";

import Link from "next/link";
import { TriangleAlert, X } from "lucide-react";
import { useState, type ReactNode } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Badge } from "@/components/ui/badge";
import { Button, buttonClasses } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { HoldToConfirm } from "@/components/ui/hold-to-confirm";
import { Checkbox, Field, Input, Select, Textarea } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { METHOD, RISK } from "@/lib/labels";
import type { AccessPolicy, DeviceCondition, NetworkCondition, PolicyConditions, PolicyEffect, PolicyMode } from "@/lib/policy-types";
import type { MethodKind } from "@/lib/types";
import { DEVICE_CONDITION, EFFECT, HALO_APP, MODE, NETWORK_CONDITION, RISK_LEVELS, describePolicy, policyState, type Names } from "./shared";

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
  risk: [],
  methods: [],
};

function nameMap(options: Option[]) {
  return Object.fromEntries(options.map((o) => [o.id, o.name]));
}

function FieldError({ children }: { children?: string }) {
  return children ? <p className="text-caption text-danger">{children}</p> : null;
}

function CheckList({ legend, options, selected, onToggle, empty }: { legend: string; options: Option[]; selected: string[]; onToggle: (id: string, on: boolean) => void; empty: ReactNode }) {
  return (
    <fieldset className="flex flex-col gap-2">
      <legend className="mb-2 text-label text-fg">{legend}</legend>
      {options.length ? (
        <div className="flex max-h-64 flex-col gap-3 overflow-y-auto rounded-md border border-border bg-sunken p-4">
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

function PeoplePicker({ label, users, selected, onChange }: { label: string; users: Option[]; selected: string[]; onChange: (ids: string[]) => void }) {
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
            <option value="">Add a person</option>
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
              {names[id] ?? "Deleted user"}
              <button
                type="button"
                aria-label={`Remove ${names[id] ?? "deleted user"}`}
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
}: {
  policy?: AccessPolicy;
  groups: Option[];
  users: Option[];
  applications: Option[];
  zones: Option[];
}) {
  const { pending, run, toast, router } = useMutation();
  const [name, setName] = useState(policy?.name ?? "");
  const [description, setDescription] = useState(policy?.description ?? "");
  const [effect, setEffect] = useState<PolicyEffect>(policy?.effect ?? "block");
  const [mode, setMode] = useState<PolicyMode>(policy?.mode ?? "report");
  const [enabled, setEnabled] = useState(policy?.enabled ?? true);
  const [c, setC] = useState<PolicyConditions>(policy?.conditions ?? BLANK);
  const [attempted, setAttempted] = useState(false);

  const apps: Option[] = [{ id: HALO_APP, name: "Halo", detail: "The admin console and the account portal" }, ...applications];
  const names: Names = { groups: nameMap(groups), users: nameMap(users), apps: nameMap(applications), zones: nameMap(zones) };
  const state = policyState({ enabled, mode });
  const problems = {
    name: name.trim() ? undefined : "Enter a name, for example “Block high-risk sign-ins”.",
    users: c.allUsers || c.groupIds.length + c.userIds.length ? undefined : "Choose at least one group or person, or apply the policy to all users.",
    apps: c.allApps || c.appIds.length ? undefined : "Choose at least one application, or cover all applications.",
    zones: c.network === "any" || c.zoneIds.length ? undefined : "Choose at least one network, or match any network.",
  };
  const invalid = Object.values(problems).some(Boolean);
  const errors: Partial<typeof problems> = attempted ? problems : {};
  const lockout = enabled && mode === "enforce" && effect !== "allow" && c.allUsers && c.excludeGroupIds.length + c.excludeUserIds.length === 0;

  function set<K extends keyof PolicyConditions>(key: K, value: PolicyConditions[K]) {
    setC((current) => ({ ...current, [key]: value }));
  }

  function toggle(key: ListKey, id: string, on: boolean) {
    setC((current) => ({ ...current, [key]: on ? [...current[key], id] : current[key].filter((v) => v !== id) }));
  }

  function save() {
    setAttempted(true);
    if (invalid) return;
    const body = { name: name.trim(), description: description.trim(), enabled, mode, effect, conditions: c };
    void run(
      "save",
      () => (policy ? api(`/policies/${policy.id}`, { method: "PUT", body }) : api("/policies", { body })),
      () => {
        toast({
          title: policy ? `${name.trim()} saved` : `${name.trim()} created`,
          description: !enabled ? "It stays off until you turn it on." : mode === "report" ? MODE.report.description : "It applies from the next sign-in.",
        });
        router.push("/admin/policies");
      },
    );
  }

  function remove() {
    if (!policy) return;
    void run("delete", () => api(`/policies/${policy.id}`, { method: "DELETE" }), () => {
      toast({ title: `${policy.name} deleted`, description: "Halo no longer evaluates it. Recorded in the audit log." });
      router.push("/admin/policies");
    });
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
          <CardHeader title="Details" />
          <CardBody className="flex flex-col gap-6">
            <Field label="Name" error={errors.name}>
              {(props) => <Input {...props} autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder="Block high-risk sign-ins" />}
            </Field>
            <Field label="Description" hint="Optional. Say why the policy exists so other administrators know whether they can change it.">
              {(props) => <Textarea {...props} rows={2} value={description} onChange={(event) => setDescription(event.target.value)} />}
            </Field>
          </CardBody>
        </Card>

        <Card>
          <CardHeader title="People" description="Who the policy applies to. Exclusions always win." />
          <CardBody className="flex flex-col gap-6">
            <Field label="Applies to">
              {(props) => (
                <Select {...props} value={c.allUsers ? "all" : "selected"} onChange={(event) => set("allUsers", event.target.value === "all")}>
                  <option value="all">All users</option>
                  <option value="selected">Selected groups and people</option>
                </Select>
              )}
            </Field>
            {c.allUsers ? null : (
              <div className="flex flex-col gap-6">
                <CheckList legend="Groups" options={groups} selected={c.groupIds} onToggle={(id, on) => toggle("groupIds", id, on)} empty="There are no groups yet." />
                <PeoplePicker label="People" users={users} selected={c.userIds} onChange={(ids) => set("userIds", ids)} />
                <FieldError>{errors.users}</FieldError>
              </div>
            )}
            <div className="flex flex-col gap-6 border-t border-border pt-6">
              <CheckList
                legend="Exclude groups"
                options={groups}
                selected={c.excludeGroupIds}
                onToggle={(id, on) => toggle("excludeGroupIds", id, on)}
                empty="There are no groups yet."
              />
              <PeoplePicker label="Exclude people" users={users} selected={c.excludeUserIds} onChange={(ids) => set("excludeUserIds", ids)} />
              <p className="text-caption text-fg-3">Exclude a break-glass group or account so an administrator can always sign in, even if a policy misfires.</p>
            </div>
          </CardBody>
        </Card>

        <Card>
          <CardHeader title="Applications" description="Where the sign-in goes. Halo itself counts as an application." />
          <CardBody className="flex flex-col gap-6">
            <Field label="Covers">
              {(props) => (
                <Select {...props} value={c.allApps ? "all" : "selected"} onChange={(event) => set("allApps", event.target.value === "all")}>
                  <option value="all">All applications, including Halo</option>
                  <option value="selected">Selected applications</option>
                </Select>
              )}
            </Field>
            {c.allApps ? null : (
              <div className="flex flex-col gap-2">
                <CheckList legend="Applications" options={apps} selected={c.appIds} onToggle={(id, on) => toggle("appIds", id, on)} empty="There are no applications yet." />
                <FieldError>{errors.apps}</FieldError>
              </div>
            )}
          </CardBody>
        </Card>

        <Card>
          <CardHeader title="Conditions" description="All of these must match. Leave a condition at its default to match everything." />
          <CardBody className="flex flex-col gap-6">
            <Field label="Network">
              {(props) => (
                <Select {...props} value={c.network} onChange={(event) => set("network", event.target.value as NetworkCondition)}>
                  {(Object.keys(NETWORK_CONDITION) as NetworkCondition[]).map((value) => (
                    <option key={value} value={value}>
                      {NETWORK_CONDITION[value]}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            {c.network === "any" ? null : (
              <div className="flex flex-col gap-2">
                <CheckList
                  legend="Networks"
                  options={zones}
                  selected={c.zoneIds}
                  onToggle={(id, on) => toggle("zoneIds", id, on)}
                  empty={
                    <>
                      There are no networks yet.{" "}
                      <Link href="/admin/policies?tab=networks" className="text-link underline underline-offset-3 hover:text-ember">
                        Add one on the Networks tab
                      </Link>
                      .
                    </>
                  }
                />
                <FieldError>{errors.zones}</FieldError>
              </div>
            )}
            <Field label="Device" hint="Administrators trust or block devices on the Devices page. Blocked devices are always refused.">
              {(props) => (
                <Select {...props} value={c.device} onChange={(event) => set("device", event.target.value as DeviceCondition)}>
                  {(Object.keys(DEVICE_CONDITION) as DeviceCondition[]).map((value) => (
                    <option key={value} value={value}>
                      {DEVICE_CONDITION[value]}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            <fieldset className="flex flex-col gap-3">
              <legend className="mb-2 text-label text-fg">Sign-in risk</legend>
              <div className="flex flex-wrap gap-x-6 gap-y-3">
                {RISK_LEVELS.map((level) => (
                  <label key={level} className="flex cursor-pointer items-center gap-2 text-body-sm text-fg">
                    <Checkbox
                      checked={c.risk.includes(level)}
                      onChange={(event) => set("risk", event.target.checked ? [...c.risk, level] : c.risk.filter((v) => v !== level))}
                    />
                    {RISK[level].label}
                  </label>
                ))}
              </div>
              <p className="text-caption text-fg-3">
                High: a risky network or 5 failed sign-ins in 15 minutes. Medium: a new device. Low: a new network for that person. None checked matches any risk.
              </p>
            </fieldset>
            <fieldset className="flex flex-col gap-3">
              <legend className="mb-2 text-label text-fg">Sign-in method</legend>
              <div className="flex flex-wrap gap-x-6 gap-y-3">
                {(Object.keys(METHOD) as MethodKind[]).map((kind) => (
                  <label key={kind} className="flex cursor-pointer items-center gap-2 text-body-sm text-fg">
                    <Checkbox
                      checked={c.methods.includes(kind)}
                      onChange={(event) => set("methods", event.target.checked ? [...c.methods, kind] : c.methods.filter((v) => v !== kind))}
                    />
                    {METHOD[kind].label}
                  </label>
                ))}
              </div>
              <p className="text-caption text-fg-3">None checked matches every method.</p>
            </fieldset>
          </CardBody>
        </Card>

        <Card>
          <CardHeader title="Effect" description="What happens when every condition matches." />
          <CardBody className="flex flex-col gap-6">
            <Field label="Effect" hint="If several enforced policies match, block beats requiring a passkey, which beats allow.">
              {(props) => (
                <Select {...props} value={effect} onChange={(event) => setEffect(event.target.value as PolicyEffect)}>
                  {(Object.keys(EFFECT) as PolicyEffect[]).map((value) => (
                    <option key={value} value={value}>
                      {EFFECT[value].label}
                    </option>
                  ))}
                </Select>
              )}
            </Field>
            <Field label="Mode" hint={MODE[mode].description}>
              {(props) => (
                <Select {...props} value={mode} onChange={(event) => setMode(event.target.value as PolicyMode)}>
                  <option value="report">Report-only</option>
                  <option value="enforce">Enforce</option>
                </Select>
              )}
            </Field>
            <label className="flex cursor-pointer items-start gap-3">
              <Checkbox className="mt-0.5" checked={enabled} onChange={(event) => setEnabled(event.target.checked)} />
              <span className="flex flex-col">
                <span className="text-body-sm text-fg">Policy is on</span>
                <span className="text-caption text-fg-3">Turn it off to keep the policy without Halo evaluating it.</span>
              </span>
            </label>
          </CardBody>
        </Card>
      </div>

      <aside className="xl:col-span-4">
        <div className="flex flex-col gap-6 xl:sticky xl:top-24">
          <Card>
            <CardHeader title="Summary" actions={<Badge tone={state.tone} dot>{state.label}</Badge>} />
            <CardBody className="flex flex-col gap-4">
              <p className="text-body text-fg">{describePolicy({ effect, conditions: c }, names)}</p>
              {lockout ? (
                <div role="note" className="flex gap-3 rounded-md border border-warning/20 bg-warning-container p-4">
                  <TriangleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0 text-warning" />
                  <p className="text-body-sm text-fg-2">This applies to everyone, including you. Exclude a break-glass group or account, or save it as report-only first.</p>
                </div>
              ) : null}
              <div className="flex flex-wrap gap-2 pt-2">
                <Button type="submit" variant="primary" loading={pending === "save"}>
                  {policy ? "Save policy" : "Create policy"}
                </Button>
                <Link href="/admin/policies" className={buttonClasses("secondary")}>
                  Cancel
                </Link>
              </div>
              {attempted && invalid ? <p className="text-caption text-danger">Fix the fields marked in red, then save again.</p> : null}
            </CardBody>
          </Card>
          {policy ? (
            <Card>
              <CardHeader title="Delete policy" description="Halo stops evaluating it immediately. Its changes stay in the audit log." />
              <CardBody>
                <HoldToConfirm size="sm" doneLabel="Deleted" onConfirm={remove}>
                  Hold to delete
                </HoldToConfirm>
              </CardBody>
            </Card>
          ) : null}
        </div>
      </aside>
    </form>
  );
}
