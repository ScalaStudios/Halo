"use client";

import { ArrowDownToLine, BookUser, ClipboardCheck, FileBadge, Network, RefreshCw, Route, Server, ToggleRight, TriangleAlert, Waypoints, Webhook, type LucideIcon } from "lucide-react";
import { useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { Checkbox } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { api } from "@/lib/api/client";
import type { Feature, FeatureId, SettingsResponse } from "@/lib/settings-types";
import { ReadOnlyNote } from "./shared";

const ICON: Partial<Record<FeatureId, LucideIcon>> = {
  saml: FileBadge,
  scim: ArrowDownToLine,
  provisioning: RefreshCw,
  ssh: Server,
  governance: ClipboardCheck,
  webhooks: Webhook,
  "forward-auth": Route,
  radius: Network,
  ldap: BookUser,
  "log-streaming": Waypoints,
};

export function FeatureSettings({ features, canEdit }: { features: Feature[]; canEdit: boolean }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [confirming, setConfirming] = useState<Feature | null>(null);

  function apply(feature: Feature, enabled: boolean, confirmed: boolean) {
    const featuresOff = features.filter((f) => (f.id === feature.id ? !enabled : !f.enabled)).map((f) => f.id);
    void run(
      feature.id,
      () => api<SettingsResponse>("/settings", { method: "PATCH", body: { featuresOff, confirmFeatures: confirmed } }),
      () => {
        setConfirming(null);
        toast({
          title: `${feature.name} turned ${enabled ? "on" : "off"}`,
          description: enabled ? "Its pages, routes and background jobs are back, with the settings it had." : "Its routes answer 404 and its background jobs stopped. Its settings are kept.",
        });
      },
      "inline",
    );
  }

  function change(feature: Feature, enabled: boolean) {
    if (!enabled && feature.usage.length > 0) setConfirming(feature);
    else apply(feature, enabled, false);
  }

  return (
    <>
      <Card>
        <ul>
          {features.map((f) => {
            const Icon = ICON[f.id] ?? ToggleRight;
            const id = `feature-${f.id}`;
            return (
              <li key={f.id} className="flex flex-col gap-4 border-b border-border px-4 py-6 last:border-b-0 sm:flex-row sm:items-start sm:justify-between sm:gap-8 sm:px-6">
                <div className="flex min-w-0 gap-4">
                  <span className="grid size-10 shrink-0 place-items-center rounded-md border border-border bg-sunken text-fg-3">
                    <Icon aria-hidden="true" size={20} strokeWidth={1.75} />
                  </span>
                  <div className="flex min-w-0 flex-col gap-1">
                    <span className="flex flex-wrap items-center gap-2">
                      <span id={id} className="font-medium text-fg">
                        {f.name}
                      </span>
                      {f.enabled ? null : <Badge tone="neutral">Off</Badge>}
                    </span>
                    <p className="text-body-sm text-fg-3">{f.description}</p>
                    {f.usage.length > 0 ? (
                      <p className={f.enabled ? "flex items-start gap-2 text-body-sm text-warning" : "tnum text-body-sm text-fg-2"}>
                        {f.enabled ? <TriangleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0" /> : null}
                        <span className="tnum">{f.usage.join(", ")}.</span>
                      </p>
                    ) : (
                      <p className="text-body-sm text-fg-2">Nothing uses it right now.</p>
                    )}
                    {confirming ? null : <ActionError message={errorFor(f.id)} />}
                  </div>
                </div>
                <label className="flex h-8 shrink-0 cursor-pointer items-center gap-2 pl-14 text-body-sm text-fg sm:pl-0">
                  {pending === f.id ? (
                    <Spinner />
                  ) : (
                    <Checkbox aria-labelledby={`${id} ${id}-state`} checked={f.enabled} disabled={!canEdit || pending !== null} onChange={(event) => change(f, event.target.checked)} />
                  )}
                  <span id={`${id}-state`}>On</span>
                </label>
              </li>
            );
          })}
        </ul>
      </Card>
      <p className="text-body-sm text-fg-3">
        Changes apply right away on this server and within 30 seconds on other Halo servers. A feature&apos;s settings are kept while it is off, so turning it back on restores them.
      </p>
      {canEdit ? null : <ReadOnlyNote>Only a global administrator can turn features on or off.</ReadOnlyNote>}
      <Dialog
        open={confirming !== null}
        onOpenChange={(open) => {
          if (open) return;
          setConfirming(null);
          clearError();
        }}
        error={confirming ? errorFor(confirming.id) : undefined}
        title={confirming ? `Turn off ${confirming.name}?` : "Turn off this feature?"}
        description={confirming ? `${confirming.usage.join(", ")}. All of this stops working until you turn it back on.` : undefined}
        footer={
          <>
            <Button variant="secondary" onClick={() => setConfirming(null)}>
              Cancel
            </Button>
            <Button variant="destructive" loading={confirming !== null && pending === confirming.id} onClick={() => confirming && apply(confirming, false, true)}>
              Turn off {confirming?.name}
            </Button>
          </>
        }
      />
    </>
  );
}
