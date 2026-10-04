"use client";

import { FlaskConical, TriangleAlert, type LucideIcon } from "lucide-react";
import { useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input, Select } from "@/components/ui/input";
import { ApiError, api } from "@/lib/api/client";
import { cn } from "@/lib/cn";
import { METHOD, RISK } from "@/lib/labels";
import type { DeviceTrust, Simulation } from "@/lib/policy-types";
import type { MethodKind } from "@/lib/types";
import { EFFECT, HALO_APP, TRUST, policyState } from "./shared";

type Option = { id: string; name: string };

const OUTCOME: Record<Simulation["outcome"], { title: string; icon: LucideIcon; box: string; color: string }> = {
  allow: { title: "Sign-in allowed", icon: EFFECT.allow.icon, box: "border-success/20 bg-success-container", color: "text-success" },
  require: { title: "Passkey or security key required", icon: EFFECT["require-phishing-resistant"].icon, box: "border-warning/20 bg-warning-container", color: "text-warning" },
  block: { title: "Sign-in blocked", icon: EFFECT.block.icon, box: "border-danger/20 bg-danger-container", color: "text-danger" },
};

function verdict(p: Simulation["policies"][number]): string {
  if (!p.matched) return "Doesn't match";
  const report = p.mode === "report";
  if (p.outcome === "block") return report ? "Would block" : "Blocks";
  if (p.outcome === "require") return report ? "Would require a passkey or security key" : "Requires a passkey or security key";
  if (p.effect === "require-phishing-resistant") return "Requirement met";
  return report ? "Would allow" : "Allows";
}

function explanation(result: Simulation): string {
  if (result.policy) {
    const met = result.outcome === "allow" && result.effect === "require-phishing-resistant" ? " This sign-in uses one." : "";
    return `“${result.policy}”: ${result.reason}${met}`;
  }
  if (result.reason) return result.reason;
  return "No enabled, enforced policy matches this sign-in.";
}

export function SimulateDialog({ users, applications }: { users: Option[]; applications: Option[] }) {
  const [open, setOpen] = useState(false);
  const [userId, setUserId] = useState("");
  const [appId, setAppId] = useState(HALO_APP);
  const [ip, setIp] = useState("");
  const [method, setMethod] = useState<MethodKind>("passkey");
  const [deviceTrust, setDeviceTrust] = useState<DeviceTrust>("unknown");
  const [errors, setErrors] = useState<{ user?: string; ip?: string; form?: string }>({});
  const [result, setResult] = useState<Simulation | null>(null);
  const [pending, setPending] = useState(false);

  async function simulate() {
    const next = {
      user: userId ? undefined : "Choose the person signing in.",
      ip: ip.trim() ? undefined : "Enter the IP address they sign in from, such as 203.0.113.7.",
    };
    setErrors(next);
    if (next.user || next.ip) return;
    setPending(true);
    try {
      setResult(await api<Simulation>("/policies/simulate", { body: { userId, appId, ip: ip.trim(), method, deviceTrust } }));
    } catch (error) {
      setResult(null);
      setErrors({ form: error instanceof ApiError ? error.message : "Halo could not reach the server. Try again." });
    } finally {
      setPending(false);
    }
  }

  const outcome = result ? OUTCOME[result.outcome] : null;
  const OutcomeIcon = outcome?.icon;

  return (
    <>
      <Button size="sm" variant="secondary" onClick={() => setOpen(true)}>
        <FlaskConical aria-hidden="true" size={16} strokeWidth={1.75} />
        Simulate sign-in
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
        width="wide"
        title="Simulate a sign-in"
        description="See which policies match a sign-in and what Halo would decide. Nothing is recorded and nobody is signed in."
        footer={
          <>
            <Button variant="secondary" onClick={() => setOpen(false)}>
              Close
            </Button>
            <Button variant="primary" loading={pending} onClick={simulate}>
              Run simulation
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
          <Field label="Person" error={errors.user}>
            {(props) => (
              <Select {...props} value={userId} onChange={(event) => setUserId(event.target.value)}>
                <option value="" disabled>
                  Choose a person
                </option>
                {users.map((u) => (
                  <option key={u.id} value={u.id}>
                    {u.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Application">
            {(props) => (
              <Select {...props} value={appId} onChange={(event) => setAppId(event.target.value)}>
                <option value={HALO_APP}>Halo console and account portal</option>
                {applications.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="IP address" error={errors.ip}>
            {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={ip} onChange={(event) => setIp(event.target.value)} placeholder="203.0.113.7" />}
          </Field>
          <Field label="Sign-in method">
            {(props) => (
              <Select {...props} value={method} onChange={(event) => setMethod(event.target.value as MethodKind)}>
                {(Object.keys(METHOD) as MethodKind[]).map((kind) => (
                  <option key={kind} value={kind}>
                    {METHOD[kind].label}
                  </option>
                ))}
              </Select>
            )}
          </Field>
          <Field label="Device" className="sm:col-span-2" hint="New devices count as unknown until an administrator trusts them.">
            {(props) => (
              <Select {...props} value={deviceTrust} onChange={(event) => setDeviceTrust(event.target.value as DeviceTrust)}>
                {(Object.keys(TRUST) as DeviceTrust[]).map((trust) => (
                  <option key={trust} value={trust}>
                    {TRUST[trust].label}
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
                  <p className="text-body-sm text-fg-2">{explanation(result)}</p>
                </div>
              </div>

              <div className="flex flex-col gap-2">
                <h3 className="flex items-center gap-2 text-h4 text-fg">
                  Sign-in risk
                  <Badge tone={RISK[result.risk].tone}>{RISK[result.risk].label}</Badge>
                </h3>
                {result.signals.length ? (
                  <ul className="flex flex-col gap-1">
                    {result.signals.map((signal) => (
                      <li key={signal.type + signal.detail} className="text-body-sm text-fg-2">
                        {signal.detail}
                      </li>
                    ))}
                  </ul>
                ) : (
                  <p className="text-body-sm text-fg-3">No risk signals for this person, network and device.</p>
                )}
              </div>

              <div className="flex flex-col gap-2">
                <h3 className="text-h4 text-fg">Policies</h3>
                {result.policies.length ? (
                  <ul className="overflow-hidden rounded-md border border-border">
                    {result.policies.map((p) => {
                      const state = policyState({ enabled: true, mode: p.mode });
                      return (
                        <li key={p.id} className="flex flex-col gap-1 border-b border-border px-4 py-3 last:border-b-0">
                          <span className="flex flex-wrap items-center gap-2">
                            <span className="text-body-sm font-medium text-fg">{p.name}</span>
                            <Badge tone={state.tone}>{state.label}</Badge>
                            <span className={cn("text-body-sm", p.matched ? "text-fg" : "text-fg-3")}>{verdict(p)}</span>
                          </span>
                          <span className="text-body-sm text-fg-3">{p.detail}</span>
                        </li>
                      );
                    })}
                  </ul>
                ) : (
                  <p className="text-body-sm text-fg-3">No policies are turned on.</p>
                )}
              </div>
            </div>
          ) : null}
        </div>
      </Dialog>
    </>
  );
}
