"use client";

import { FlaskConical, TriangleAlert, type LucideIcon } from "lucide-react";
import { useState, type ReactNode } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input, Select } from "@/components/ui/input";
import { ApiError, api } from "@/lib/api/client";
import { cn } from "@/lib/cn";
import { useMessages } from "@/lib/i18n/client";
import { frameMessages } from "@/lib/i18n/console/frame";
import { policyMessages } from "@/lib/i18n/console/policy";
import { RISK } from "@/lib/labels";
import type { ComplianceState, DeviceTrust, Simulation } from "@/lib/policy-types";
import type { MethodKind } from "@/lib/types";
import { EFFECT, HALO_APP, POLICY_METHODS, policyState, type PolicyCopy } from "./shared";

type Option = { id: string; name: string };

const OUTCOME: Record<Simulation["outcome"], { icon: LucideIcon; box: string; color: string }> = {
  allow: { icon: EFFECT.allow.icon, box: "border-success/20 bg-success-container", color: "text-success" },
  require: { icon: EFFECT["require-phishing-resistant"].icon, box: "border-warning/20 bg-warning-container", color: "text-warning" },
  terms: { icon: EFFECT["require-terms"].icon, box: "border-info/20 bg-info-container", color: "text-info" },
  block: { icon: EFFECT.block.icon, box: "border-danger/20 bg-danger-container", color: "text-danger" },
};

function verdict(p: Simulation["policies"][number], t: PolicyCopy["simulate"]["verdict"]): string {
  if (!p.matched) return t.noMatch;
  const report = p.mode === "report";
  if (p.outcome === "block") return t.block(report);
  if (p.outcome === "require" && p.effect === "require-context") return t.context(report);
  if (p.outcome === "require") return t.require(report);
  if (p.outcome === "terms") return t.terms(report);
  if (p.effect === "require-phishing-resistant") return t.met;
  if (p.effect === "require-terms") return t.accepted;
  if (p.effect === "require-context") return t.contextMet;
  return t.allow(report);
}

function explanation(result: Simulation, t: PolicyCopy): ReactNode {
  if (result.policy) {
    const met = result.outcome === "allow" && result.effect === "require-phishing-resistant" ? ` ${t.simulate.usesOne}` : "";
    return (
      <>
        {t.describe.quote(result.policy)}: <span lang="en">{result.reason}</span>
        {met}
      </>
    );
  }
  if (result.reason) return <span lang="en">{result.reason}</span>;
  return t.simulate.noPolicy;
}

export function SimulateDialog({ users, applications }: { users: Option[]; applications: Option[] }) {
  const copy = useMessages(policyMessages);
  const unreachable = useMessages(frameMessages).mutation.unreachable;
  const t = copy.simulate;
  const [open, setOpen] = useState(false);
  const [userId, setUserId] = useState("");
  const [appId, setAppId] = useState(HALO_APP);
  const [ip, setIp] = useState("");
  const [method, setMethod] = useState<MethodKind>("passkey");
  const [deviceTrust, setDeviceTrust] = useState<DeviceTrust>("unknown");
  const [deviceCompliance, setDeviceCompliance] = useState<ComplianceState>("unknown");
  const [errors, setErrors] = useState<{ user?: string; ip?: string; form?: ReactNode }>({});
  const [result, setResult] = useState<Simulation | null>(null);
  const [pending, setPending] = useState(false);

  async function simulate() {
    const next = {
      user: userId ? undefined : t.personRequired,
      ip: ip.trim() ? undefined : t.ipRequired,
    };
    setErrors(next);
    if (next.user || next.ip) return;
    setPending(true);
    try {
      setResult(await api<Simulation>("/policies/simulate", { body: { userId, appId, ip: ip.trim(), method, deviceTrust, deviceCompliance } }));
    } catch (error) {
      setResult(null);
      setErrors({ form: error instanceof ApiError ? <span lang="en">{error.message}</span> : unreachable });
    } finally {
      setPending(false);
    }
  }

  const outcome = result ? { ...OUTCOME[result.outcome], title: result.outcome === "require" && result.effect !== "require-phishing-resistant" ? t.stronger : t.outcome[result.outcome] } : null;
  const OutcomeIcon = outcome?.icon;

  return (
    <>
      <Button size="sm" variant="secondary" onClick={() => setOpen(true)}>
        <FlaskConical aria-hidden="true" size={16} strokeWidth={1.75} />
        {t.open}
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        width="wide"
        title={t.title}
        description={t.description}
        footer={
          <>
            <Button variant="secondary" onClick={() => setOpen(false)}>
              {t.close}
            </Button>
            <Button variant="primary" loading={pending} onClick={simulate}>
              {t.run}
            </Button>
          </>
        }
      >
        <form
          className="grid grid-cols-1 gap-6 sm:grid-cols-2"
          onSubmit={(event) => {
            event.preventDefault();
            void simulate();
          }}
        >
          <Field label={t.person} error={errors.user}>
            {(props) => (
              <Select {...props} value={userId} onChange={(event) => setUserId(event.target.value)}>
                <option value="" disabled>
                  {t.choosePerson}
                </option>
                {users.map((u) => (
                  <option key={u.id} value={u.id}>
                    {u.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label={t.application}>
            {(props) => (
              <Select {...props} value={appId} onChange={(event) => setAppId(event.target.value)}>
                <option value={HALO_APP}>{t.halo}</option>
                {applications.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label={t.ip} error={errors.ip}>
            {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={ip} onChange={(event) => setIp(event.target.value)} placeholder="203.0.113.7" />}
          </Field>
          <Field label={t.method}>
            {(props) => (
              <Select {...props} value={method} onChange={(event) => setMethod(event.target.value as MethodKind)}>
                {POLICY_METHODS.map((kind) => (
                  <option key={kind} value={kind}>
                    {copy.labels.method[kind]}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label={t.trust} hint={t.trustHint}>
            {(props) => (
              <Select {...props} value={deviceTrust} onChange={(event) => setDeviceTrust(event.target.value as DeviceTrust)}>
                {(Object.keys(copy.labels.trust) as DeviceTrust[]).map((trust) => (
                  <option key={trust} value={trust}>
                    {copy.labels.trust[trust]}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label={t.compliance} hint={t.complianceHint}>
            {(props) => (
              <Select {...props} value={deviceCompliance} onChange={(event) => setDeviceCompliance(event.target.value as ComplianceState)}>
                {(Object.keys(copy.labels.compliance) as ComplianceState[]).map((state) => (
                  <option key={state} value={state}>
                    {copy.labels.compliance[state]}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <button type="submit" hidden />
        </form>

        <div aria-live="polite" className="mt-8 flex flex-col gap-6 pb-4">
          {errors.form ? (
            <p role="alert" className="flex gap-3 rounded-md border border-danger/20 bg-danger-container p-4 text-body-sm text-fg">
              <TriangleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0 text-danger" />
              {errors.form}
            </p>
          ) : null}
          {result && outcome && OutcomeIcon ? (
            <div key={JSON.stringify(result)} className="flex animate-enter flex-col gap-6">
              <div className={cn("flex gap-3 rounded-md border p-4", outcome.box)}>
                <OutcomeIcon aria-hidden="true" size={20} strokeWidth={1.75} className={cn("mt-px shrink-0", outcome.color)} />
                <div className="flex min-w-0 flex-col gap-1">
                  <p className="text-body-sm font-semibold text-fg">{outcome.title}</p>
                  <p className="text-body-sm text-fg-2">{explanation(result, copy)}</p>
                </div>
              </div>

              <div className="flex flex-col gap-2">
                <h3 className="flex items-center gap-2 text-h4 text-fg">
                  {t.risk}
                  <Badge tone={RISK[result.risk].tone}>{copy.labels.risk[result.risk]}</Badge>
                </h3>
                {result.signals.length ? (
                  <ul className="flex flex-col gap-1">
                    {result.signals.map((signal) => (
                      <li key={signal.type + signal.detail} lang="en" className="text-body-sm text-fg-2">
                        {signal.detail}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="text-body-sm text-fg-3">{t.noSignals}</p>
                )}
              </div>

              <div className="flex flex-col gap-2">
                <h3 className="text-h4 text-fg">{t.policies}</h3>
                {result.policies.length ? (
                  <ul className="overflow-hidden rounded-md border border-border">
                    {result.policies.map((p) => {
                      const state = policyState({ enabled: true, mode: p.mode }, copy);
                      return (
                        <li key={p.id} className="flex flex-col gap-1 border-b border-border px-4 py-3 last:border-b-0">
                          <span className="flex flex-wrap items-center gap-2">
                            <span className="text-body-sm font-medium text-fg">{p.name}</span>
                            <Badge tone={state.tone}>{state.label}</Badge>
                            <span className={cn("text-body-sm", p.matched ? "text-fg" : "text-fg-3")}>{verdict(p, t.verdict)}</span>
                          </span>
                          <span lang="en" className="text-body-sm text-fg-3">{p.detail}</span>
                        </li>
                      );
                    })}
                  </ul>
                ) : (
                  <p className="text-body-sm text-fg-3">{t.noPolicies}</p>
                )}
              </div>
            </div>
          ) : null}
        </div>
      </Dialog>
    </>
  );
}
