"use client";

import Link from "next/link";
import { ArrowDown, ArrowUp, FlaskConical, MoreHorizontal, Pencil, Plus, Power, Shield, ShieldCheck, Trash2 } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Badge } from "@/components/ui/badge";
import { Button, buttonClasses } from "@/components/ui/button";
import { Card, CardHeader } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Menu, MenuContent, MenuItem, MenuSeparator, MenuTrigger } from "@/components/ui/menu";
import { api } from "@/lib/api/client";
import { useMessages } from "@/lib/i18n/client";
import { policyMessages } from "@/lib/i18n/console/policy";
import type { PolicyWithActivity } from "@/lib/policy-types";
import { activityText, describePolicy, policyState, type Names } from "./shared";

function payload(p: PolicyWithActivity, change: { enabled?: boolean; mode?: PolicyWithActivity["mode"] }) {
  return { name: p.name, description: p.description, enabled: p.enabled, mode: p.mode, effect: p.effect, termsId: p.termsId, contextId: p.contextId, conditions: p.conditions, ...change };
}

export function ReportInsights({ policies }: { policies: PolicyWithActivity[] }) {
  const copy = useMessages(policyMessages);
  const report = policies.filter((p) => p.enabled && p.mode === "report");
  if (report.length === 0) return null;
  return (
    <Card>
      <CardHeader title={copy.list.insights} description={copy.list.insightsDescription} />
      <ul className="border-t border-border">
        {report.map((p) => (
          <li key={p.id} className="flex flex-col gap-3 border-b border-border px-6 py-4 last:border-b-0 sm:flex-row sm:items-center sm:justify-between sm:gap-6">
            <div className="flex min-w-0 flex-col gap-1">
              <span className="text-body-sm font-medium text-fg">{p.name}</span>
              <span className="text-body-sm text-fg-2">{activityText(p, copy)}</span>
            </div>
            <Link href={`/admin/policies/${p.id}`} className={buttonClasses("secondary", "sm", "self-start sm:self-center")}>
              {copy.list.review}
            </Link>
          </li>
        ))}
      </ul>
    </Card>
  );
}

export function PolicyList({ policies, names }: { policies: PolicyWithActivity[]; names: Names }) {
  const copy = useMessages(policyMessages);
  const t = copy.list;
  const { pending, run, errorFor, clearError, toast, router } = useMutation();
  const [deleting, setDeleting] = useState<PolicyWithActivity | null>(null);

  function move(index: number, delta: number) {
    const ids = policies.map((p) => p.id);
    const target = index + delta;
    [ids[index], ids[target]] = [ids[target]!, ids[index]!];
    const name = policies[index]!.name;
    void run("order", () => api("/policies/order", { method: "PUT", body: { ids } }), () =>
      toast({ title: t.orderSaved, description: t.position(name, target + 1) }),
    );
  }

  function update(p: PolicyWithActivity, change: { enabled?: boolean; mode?: PolicyWithActivity["mode"] }, title: string, description: string) {
    void run(p.id, () => api(`/policies/${p.id}`, { method: "PUT", body: payload(p, change) }), () => toast({ title, description }));
  }

  function remove() {
    const p = deleting;
    if (!p) return;
    void run("delete", () => api(`/policies/${p.id}`, { method: "DELETE" }), () => {
      setDeleting(null);
      toast({ title: copy.deleted(p.name), description: t.deletedDescription });
    }, "inline");
  }

  if (policies.length === 0) {
    return (
      <Card>
        <EmptyState
          icon={Shield}
          title={t.empty}
          description={t.emptyDescription}
          action={
            <Link href="/admin/policies/new" className={buttonClasses("primary", "sm")}>
              <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
              {copy.page.create}
            </Link>
          }
        />
      </Card>
    );
  }

  return (
    <>
      <Card>
        <ol aria-label={t.order}>
          {policies.map((p, index) => {
            const state = policyState(p, copy);
            return (
              <li key={p.id} className="flex items-start gap-4 border-b border-border px-4 py-4 last:border-b-0 sm:px-6">
                <span className="tnum w-6 shrink-0 pt-px text-body-sm text-fg-3">{index + 1}</span>
                <div className="flex min-w-0 flex-1 flex-col gap-1">
                  <div className="flex flex-wrap items-center gap-3">
                    <Link href={`/admin/policies/${p.id}`} className="font-medium text-fg underline-offset-3 hover:underline">
                      {p.name}
                    </Link>
                    <Badge tone={state.tone} dot>
                      {state.label}
                    </Badge>
                  </div>
                  <p className="text-body-sm text-fg-2">{describePolicy(p, names, copy)}</p>
                  <p className="text-caption text-fg-3">{activityText(p, copy)}</p>
                </div>
                <div className="flex shrink-0 items-center gap-1">
                  <Button size="icon-sm" variant="quiet" aria-label={t.moveUp(p.name)} disabled={index === 0 || pending === "order"} onClick={() => move(index, -1)}>
                    <ArrowUp aria-hidden="true" size={16} strokeWidth={1.75} />
                  </Button>
                  <Button
                    size="icon-sm"
                    variant="quiet"
                    aria-label={t.moveDown(p.name)}
                    disabled={index === policies.length - 1 || pending === "order"}
                    onClick={() => move(index, 1)}
                  >
                    <ArrowDown aria-hidden="true" size={16} strokeWidth={1.75} />
                  </Button>
                  <Menu>
                    <MenuTrigger asChild>
                      <Button size="icon-sm" variant="quiet" aria-label={t.actionsFor(p.name)} loading={pending === p.id}>
                        <MoreHorizontal aria-hidden="true" size={16} strokeWidth={1.75} />
                      </Button>
                    </MenuTrigger>
                    <MenuContent>
                      <MenuItem icon={Pencil} onSelect={() => router.push(`/admin/policies/${p.id}`)}>
                        {t.edit}
                      </MenuItem>
                      {p.mode === "report" ? (
                        <MenuItem icon={ShieldCheck} onSelect={() => update(p, { mode: "enforce" }, t.enforced(p.name), t.enforcedDescription)}>
                          {t.enforce}
                        </MenuItem>
                      ) : (
                        <MenuItem
                          icon={FlaskConical}
                          onSelect={() => update(p, { mode: "report" }, t.reportOnly(p.name), t.reportOnlyDescription)}
                        >
                          {t.switchToReport}
                        </MenuItem>
                      )}
                      <MenuItem
                        icon={Power}
                        onSelect={() =>
                          p.enabled
                            ? update(p, { enabled: false }, t.turnedOff(p.name), t.turnedOffDescription)
                            : update(p, { enabled: true }, t.turnedOn(p.name), p.mode === "enforce" ? t.turnedOnEnforced : t.turnedOnReport)
                        }
                      >
                        {p.enabled ? t.turnOff : t.turnOn}
                      </MenuItem>
                      <MenuSeparator />
                      <MenuItem icon={Trash2} danger onSelect={() => setDeleting(p)}>
                        {t.delete}
                      </MenuItem>
                    </MenuContent>
                  </Menu>
                </div>
              </li>
            );
          })}
        </ol>
      </Card>
      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (open) return;
          setDeleting(null);
          clearError();
        }}
        error={errorFor("delete")}
        title={copy.confirmDelete(deleting?.name ?? t.deleteFallback)}
        description={t.deleteDescription}
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              {copy.cancel}
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              {t.delete}
            </Button>
          </>
        }
      />
    </>
  );
}
