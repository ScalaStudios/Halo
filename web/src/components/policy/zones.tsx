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
import { pluralize } from "@/lib/format";
import type { NetworkZone } from "@/lib/policy-types";
import { ZONE_KIND, joinList } from "./shared";

export function ZonesSection({ zones, usedBy }: { zones: NetworkZone[]; usedBy: Record<string, string[]> }) {
  const { pending, run, toast } = useMutation();
  const [editing, setEditing] = useState<NetworkZone | "new" | null>(null);
  const [deleting, setDeleting] = useState<NetworkZone | null>(null);

  function remove() {
    const zone = deleting;
    if (!zone) return;
    void run("delete", () => api(`/network-zones/${zone.id}`, { method: "DELETE" }), () => {
      setDeleting(null);
      toast({ title: `${zone.name} deleted`, description: "Recorded in the audit log." });
    });
  }

  return (
    <div className="flex flex-col gap-6">
      <SectionTitle
        title="Networks"
        description="Named IP ranges that policies can target. Sign-ins from a risky network are rated high risk."
        action={
          <Button size="sm" variant="primary" onClick={() => setEditing("new")}>
            <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
            Add network
          </Button>
        }
      />
      {zones.length ? (
        <SimpleTable
          caption="Networks"
          head={["Name", "Type", "Ranges", "Used by", "Actions"]}
          rows={zones.map((zone) => [
            <span key="name" className="font-medium whitespace-nowrap text-fg">
              {zone.name}
            </span>,
            <Badge key="kind" tone={ZONE_KIND[zone.kind].tone} dot>
              {ZONE_KIND[zone.kind].label}
            </Badge>,
            <span key="ranges" className="flex flex-wrap items-center gap-1">
              {zone.cidrs.slice(0, 3).map((cidr) => (
                <Tag key={cidr}>{cidr}</Tag>
              ))}
              {zone.cidrs.length > 3 ? <span className="text-caption text-fg-3">and {zone.cidrs.length - 3} more</span> : null}
            </span>,
            <span key="used" className="flex flex-col gap-1">
              {usedBy[zone.id]?.length ? <span>{joinList(usedBy[zone.id]!, "and")}</span> : null}
              {zone.kind === "risky" ? <span className="text-caption text-fg-3">Rates sign-ins from it high risk</span> : null}
              {!usedBy[zone.id]?.length && zone.kind === "trusted" ? <span className="text-fg-3">No policy</span> : null}
            </span>,
            <span key="actions" className="flex gap-1">
              <Button size="icon-sm" variant="quiet" aria-label={`Edit ${zone.name}`} onClick={() => setEditing(zone)}>
                <Pencil aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
              <Button size="icon-sm" variant="quiet" aria-label={`Delete ${zone.name}`} onClick={() => setDeleting(zone)}>
                <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
              </Button>
            </span>,
          ])}
        />
      ) : (
        <Card>
          <EmptyState
            icon={Network}
            title="No networks yet"
            description="Add your offices and VPN as trusted networks, and known-bad ranges such as Tor exit nodes as risky ones."
          />
        </Card>
      )}
      {editing ? <ZoneDialog key={editing === "new" ? "new" : editing.id} zone={editing === "new" ? null : editing} onClose={() => setEditing(null)} /> : null}
      <Dialog
        open={deleting !== null}
        onOpenChange={(open) => {
          if (!open) setDeleting(null);
        }}
        title={`Delete ${deleting?.name ?? "network"}?`}
        description="Policies can no longer target it. Halo refuses if a policy still uses it."
        footer={
          <>
            <Button variant="secondary" onClick={() => setDeleting(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={pending === "delete"} onClick={remove}>
              Delete network
            </Button>
          </>
        }
      />
    </div>
  );
}

function ZoneDialog({ zone, onClose }: { zone: NetworkZone | null; onClose: () => void }) {
  const { pending, run, toast } = useMutation();
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
      name: name.trim() ? undefined : "Enter a name, for example “Office”.",
      ranges: cidrs.length ? undefined : "Add at least one IP address or range, such as 203.0.113.0/24.",
    };
    setErrors(next);
    if (next.name || next.ranges) return;
    const body = { name: name.trim(), kind, cidrs };
    void run(
      "save",
      () => (zone ? api<NetworkZone>(`/network-zones/${zone.id}`, { method: "PUT", body }) : api<NetworkZone>("/network-zones", { body })),
      (saved) => {
        toast({ title: zone ? `${saved.name} saved` : `${saved.name} added`, description: `${pluralize(saved.cidrs.length, "range")}. Policies that use it apply the change right away.` });
        onClose();
      },
    );
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
      title={zone ? `Edit ${zone.name}` : "Add network"}
      description="Halo matches the sign-in IP address against these ranges."
      footer={
        <>
          <Button variant="secondary" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" loading={pending === "save"} onClick={save}>
            {zone ? "Save network" : "Add network"}
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
        <Field label="Name" error={errors.name}>
          {(props) => <Input {...props} autoFocus autoComplete="off" value={name} onChange={(event) => setName(event.target.value)} placeholder="Office" />}
        </Field>
        <Field label="Type" hint={ZONE_KIND[kind].description}>
          {(props) => (
            <Select {...props} value={kind} onChange={(event) => setKind(event.target.value as NetworkZone["kind"])}>
              <option value="trusted">Trusted</option>
              <option value="risky">Risky</option>
            </Select>
          )}
        </Field>
        <Field label="IP addresses and ranges" error={errors.ranges} hint="One per line, IPv4 or IPv6, such as 203.0.113.0/24 or 2001:db8::/48. A single address counts as one host.">
          {(props) => <Textarea {...props} mono rows={5} spellCheck={false} value={ranges} onChange={(event) => setRanges(event.target.value)} placeholder="203.0.113.0/24" />}
        </Field>
        <button type="submit" hidden />
      </form>
    </Dialog>
  );
}
