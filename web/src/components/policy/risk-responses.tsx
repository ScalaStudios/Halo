"use client";

import { Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { ActionError } from "@/components/ui/action-error";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Select } from "@/components/ui/input";
import { FieldError } from "@/components/policy/policy-form";
import { ReadOnlyNote, useSaveSettings } from "@/components/settings/shared";
import type { RiskEventType } from "@/lib/policy-types";
import type { RiskResponse, Settings } from "@/lib/settings-types";
import { RISK_TYPE } from "./shared";

const ACTION: Record<RiskResponse["action"], string> = {
  "revoke-sessions": "End their sessions and revoke tokens",
  "require-passkey": "End their sessions and require a passkey",
  suspend: "Suspend the account",
  notify: "Email the person",
};

export function RiskResponsesForm({ settings, canEdit }: { settings: Settings; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const [rows, setRows] = useState(settings.riskResponses);
  const [attempted, setAttempted] = useState(false);
  const dirty = JSON.stringify(rows) !== JSON.stringify(settings.riskResponses);
  const error = rows.some((row, i) => rows.findIndex((other) => JSON.stringify(other) === JSON.stringify(row)) !== i)
    ? "Each response must be different. Remove the repeated one."
    : undefined;

  function update(index: number, change: Partial<RiskResponse>) {
    setRows((current) => current.map((row, i) => (i === index ? { ...row, ...change } : row)));
  }

  function submit() {
    setAttempted(true);
    if (error) return;
    void save({ riskResponses: rows }, "Automatic responses saved", "New risk events get them from now on. Earlier risk events are not acted on again.");
  }

  return (
    <Card>
      <CardHeader
        title="Automatic responses"
        description="What Halo does on its own within seconds of a risk event, at any hour. Each event is acted on once and recorded in the audit log as risk_event.respond, which notification rules can send on. Emergency access accounts are never touched."
      />
      <CardBody>
        <form
          noValidate
          className="flex flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <fieldset className="flex flex-col gap-3">
            <legend className="sr-only">Responses</legend>
            {rows.length === 0 ? <p className="text-body-sm text-fg-3">No automatic responses. Risk events wait for a security administrator.</p> : null}
            {rows.map((row, index) => (
              <div key={index} className="flex flex-wrap items-center gap-3">
                <div className="w-52">
                  <Select aria-label={`Signal of response ${index + 1}`} disabled={!canEdit} value={row.signal} onChange={(event) => update(index, { signal: event.target.value as RiskResponse["signal"] })}>
                    <option value="">Any risk event</option>
                    {(Object.keys(RISK_TYPE) as RiskEventType[]).map((type) => (
                      <option key={type} value={type}>
                        {RISK_TYPE[type]}
                      </option>
                    ))}
                  </Select>
                </div>
                <div className="w-44">
                  <Select aria-label={`Level of response ${index + 1}`} disabled={!canEdit} value={row.level} onChange={(event) => update(index, { level: event.target.value as RiskResponse["level"] })}>
                    <option value="medium">Medium or high risk</option>
                    <option value="high">High risk</option>
                  </Select>
                </div>
                <div className="w-72">
                  <Select aria-label={`Action of response ${index + 1}`} disabled={!canEdit} value={row.action} onChange={(event) => update(index, { action: event.target.value as RiskResponse["action"] })}>
                    {(Object.keys(ACTION) as RiskResponse["action"][]).map((action) => (
                      <option key={action} value={action}>
                        {ACTION[action]}
                      </option>
                    ))}
                  </Select>
                </div>
                {canEdit ? (
                  <Button size="icon-sm" variant="quiet" aria-label={`Remove response ${index + 1}`} onClick={() => setRows((current) => current.filter((_, i) => i !== index))}>
                    <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                  </Button>
                ) : null}
              </div>
            ))}
            {canEdit ? (
              <div>
                <Button size="sm" variant="secondary" disabled={rows.length >= 20} onClick={() => setRows((current) => [...current, { signal: "", level: "high", action: "revoke-sessions" }])}>
                  <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
                  Add response
                </Button>
              </div>
            ) : null}
            <FieldError>{attempted ? error : undefined}</FieldError>
          </fieldset>
          {canEdit ? (
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                Save responses
              </Button>
              <ActionError message={failure} />
            </div>
          ) : (
            <ReadOnlyNote>Only security and global administrators can change automatic responses.</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}
