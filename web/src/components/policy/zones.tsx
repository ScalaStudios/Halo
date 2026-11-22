"use client";

import { Network, Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Badge, Tag } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { EmptyState } from "@/components/ui/empty-state";
import { Field, Input, Select, Textarea } from "@/components/ui/input";
import { SectionTitle } from "@/components/ui/section-title";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { useMessages } from "@/lib/i18n/client";
import { policyMessages } from "@/lib/i18n/console/policy";
import type { NetworkZone } from "@/lib/policy-types";
import { ZONE_KIND, joinList } from "./shared";

export function ZonesSection({ zones, usedBy }: { zones: NetworkZone[]; usedBy: Record<string, string[]> }) {
  const copy = useMessages(policyMessages);
  const t = copy.zones;
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [editing, setEditing] = useState<NetworkZone | "new" | null>(null);
  const [deleting, setDeleting] = useState<NetworkZone | null>(null);

  function remove() {
    const zone = deleting;
    if (!zone) return;
    void run("delete", () => api(`/network-zones/${zone.id}`, { method: "DELETE" }), () => {
      setDeleting(null);
      toast({ title: copy.deleted(zone.name), description: copy.recorded });
    }, "inline");
  }

  return (
    <div className="flex flex-col gap-6">
      <SectionTitle
        title={t.title}
        description={t.description}
        action={
          <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            {t.add}
          </Button>
        }
      />
      {zones.length ? (
        <SimpleTable
          caption={t.title}
          head={[copy.name, copy.type, t.ranges, copy.usedBy, copy.actions]}
          rows={zones.map((zone) => [
            <span key="name" className="font-medium whitespace-nowrap text-fg">
              {zone.name}
            </span>,
            <Badge key="kind" tone={ZONE_KIND[zone.kind].tone} dot>
              {copy.labels.zoneKind[zone.kind].label}
            </Badge>,
            <span key="ranges" className="flex flex-wrap items-center gap-1">
              {zone.cidrs.slice(0, 3).map((cidr) => (
                <Tag key={cidr}>{cidr}</Tag>
              ))}
              {zone.cidrs.length > 3 ? <span className="text-caption text-fg-3">{t.more(zone.cidrs.length - 3)}</span> : null}
            </span>,
            <span key="used" className="flex flex-col gap-1">
              {usedBy[zone.id]?.length ? <span>{joinList(usedBy[zone.id]!, copy.and)}</span> : null}
              {zone.kind === "risky" ? <span className="text-caption text-fg-3">{t.ratesHigh}</span> : null}
              {!usedBy[zone.id]?.length && zone.kind === "trusted" ? <span className="text-fg-3">{copy.noPolicy}</span> : null}
            </span>,
            <span key="actions" className="flex gap-1">
              <Button size="icon-sm" variant="quiet" aria-label={copy.edit(zone.name)} onClick={() => setEditing(zone)}>
                <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
              <Button size="icon-sm" variant="quiet" aria-label={copy.delete(zone.name)} onClick={() => setDeleting(zone)}>
                <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
            </span>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={Network}
            title={t.empty}
            description={t.emptyDescription}
          />
        </Card>
      )}
      {editing ? <ZoneDialog key={editing === "new" ? "new" : editing.id} zone={editing === "new" ? null : editing} onClose={() => setEditing(null)} /> : null}
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
    </div>
  );
}

function ZoneDialog({ zone, onClose }: { zone: NetworkZone | null; onClose: () => void }) {
  const copy = useMessages(policyMessages);
  const t = copy.zones;
  const { pending, run, errorFor, toast } = useMutation();
  const [name, setName] = useState(zone?.name ?? "");
  const [kind, setKind] = useState<NetworkZone["kind"]>(zone?.kind ?? "trusted");
  const [ranges, setRanges] = useState(zone?.cidrs.join("\n") ?? "");
  const [errors, setErrors] = useState<{ name?: string; ranges?: string }>({});

  function save() {
    const cidrs = ranges
      .split(/[\s,]+/)
      .map((value) => value.trim())
      .filter(Boolean);
    const next = {
      name: name.trim() ? undefined : copy.enterName(t.nameExample),
      ranges: cidrs.length ? undefined : t.rangesRequired,
    };
    setErrors(next);
    if (next.name || next.ranges) return;
    const body = { name: name.trim(), kind, cidrs };
    void run(
      "save",
      () => (zone ? api<NetworkZone>(`/network-zones/${zone.id}`, { method: "PUT", body }) : api<NetworkZone>("/network-zones", { body })),
      (saved) => {
        toast({ title: zone ? copy.saved(saved.name) : copy.added(saved.name), description: t.savedDescription(saved.cidrs.length) });
        onClose();
      },
      "inline",
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      error={errorFor("save")}
      title={zone ? copy.edit(zone.name) : t.add}
      description={t.dialogDescription}
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            {copy.cancel}
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {zone ? t.save : t.add}
          </Button>
        </>
      }
    >
      <form
        className="flex flex-col gap-6"
        onSubmit={(event) => {
          event.preventDefault();
          save();
        }}
      >
        <Field label={copy.name} error={errors.name}>
          {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder={t.nameExample} />}
        </Field>
        <Field label={copy.type} hint={copy.labels.zoneKind[kind].description}>
          {(props) => (
            <Select {...props} value={kind} onChange={(event) => setKind(event.target.value as NetworkZone["kind"])}>
              <option value="trusted">{copy.labels.zoneKind.trusted.label}</option>
              <option value="risky">{copy.labels.zoneKind.risky.label}</option>
            </Select>
          )}
        </Field>
        <Field label={t.addresses} error={errors.ranges} hint={t.addressesHint}>
          {(props) => <Textarea {...props} mono rows={5} spellCheck={false} value={ranges} onChange={(event) => setRanges(event.target.value)} placeholder="203.0.113.0/24" />}
        </Field>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
