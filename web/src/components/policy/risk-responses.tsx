"use client";

import { Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { ActionError } from "@/components/ui/action-error";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Select } from "@/components/ui/input";
import { FieldError } from "@/components/policy/policy-form";
import { ReadOnlyNote, useSaveSettings } from "@/components/settings/shared";
import { useMessages } from "@/lib/i18n/client";
import { policyMessages } from "@/lib/i18n/console/policy";
import type { RiskEventType } from "@/lib/policy-types";
import type { RiskResponse, Settings } from "@/lib/settings-types";

export function RiskResponsesForm({ settings, canEdit }: { settings: Settings; canEdit: boolean }) {
  const copy = useMessages(policyMessages);
  const t = copy.responses;
  const { saving, failure, save } = useSaveSettings();
  const [rows, setRows] = useState(settings.riskResponses);
  const [attempted, setAttempted] = useState(false);
  const dirty = JSON.stringify(rows) !== JSON.stringify(settings.riskResponses);
  const error = rows.some((row, i) => rows.findIndex((other) => JSON.stringify(other) === JSON.stringify(row)) !== i)
    ? t.duplicate
    : undefined;

  function update(index: number, change: Partial<RiskResponse>) {
    setRows((current) => current.map((row, i) => (i === index ? { ...row, ...change } : row)));
  }

  function submit() {
    setAttempted(true);
    if (error) return;
    void save({ riskResponses: rows }, t.saved, t.savedDescription);
  }

  return (
    <Card>
      <CardHeader
        title={t.title}
        description={t.description}
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
            <legend className="sr-only">{t.legend}</legend>
            {rows.length === 0 ? <p className="text-body-sm text-fg-3">{t.empty}</p> : null}
            {rows.map((row, index) => (
              <div key={index} className="flex flex-wrap items-center gap-3">
                <div className="w-52">
                  <Select aria-label={t.signal(index + 1)} disabled={!canEdit} value={row.signal} onChange={(event) => update(index, { signal: event.target.value as RiskResponse["signal"] })}>
                    <option value="">{t.anyEvent}</option>
                    {(Object.keys(copy.labels.riskType) as RiskEventType[]).map((type) => (
                      <option key={type} value={type}>
                        {copy.labels.riskType[type]}
                      </option>
                    ))}
                  </Select>
                </div>
                <div className="w-44">
                  <Select aria-label={t.level(index + 1)} disabled={!canEdit} value={row.level} onChange={(event) => update(index, { level: event.target.value as RiskResponse["level"] })}>
                    <option value="medium">{t.mediumOrHigh}</option>
                    <option value="high">{t.high}</option>
                  </Select>
                </div>
                <div className="w-72">
                  <Select aria-label={t.actionLabel(index + 1)} disabled={!canEdit} value={row.action} onChange={(event) => update(index, { action: event.target.value as RiskResponse["action"] })}>
                    {(Object.keys(t.action) as RiskResponse["action"][]).map((action) => (
                      <option key={action} value={action}>
                        {t.action[action]}
                      </option>
                    ))}
                  </Select>
                </div>
                {canEdit ? (
                  <Button size="icon-sm" variant="quiet" aria-label={t.removeLabel(index + 1)} onClick={() => setRows((current) => current.filter((_, i) => i !== index))}>
                    <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
                  </Button>
                ) : null}
              </div>
            ))}
            {canEdit ? (
              <div>
                <Button size="sm" variant="secondary" disabled={rows.length >= 20} onClick={() => setRows((current) => [...current, { signal: "", level: "high", action: "revoke-sessions" }])}>
                  <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
                  {t.add}
                </Button>
              </div>
            ) : null}
            <FieldError>{attempted ? error : undefined}</FieldError>
          </fieldset>
          {canEdit ? (
            <div className="flex flex-wrap items-center gap-4">
              <Button type="submit" variant="primary" loading={saving} disabled={!dirty}>
                {t.save}
              </Button>
              <ActionError message={failure} />
            </div>
          ) : (
            <ReadOnlyNote>{t.readOnly}</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}
