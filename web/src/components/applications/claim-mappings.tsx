"use client";

import { Plus, Trash2 } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import { useMessages } from "@/lib/i18n/client";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import type { Attribute, Claim } from "@/lib/types";

const PLACEHOLDER = { claim: "employee_id", attribute: "employeeNumber", header: "X-Employee-Id" } as const;

export function ClaimMappingsForm({
  appId,
  appName,
  kind,
  mappings,
  attributes,
}: {
  appId: string;
  appName: string;
  kind: keyof typeof PLACEHOLDER;
  mappings: Claim[];
  attributes: Attribute[];
}) {
  const [rows, setRows] = useState<Claim[]>(mappings);
  const { pending, run, errorFor, toast } = useMutation();
  const t = useMessages(applicationsMessages);
  const m = t.mappings;
  const k = m.kinds[kind];
  const update = (index: number, change: Partial<Claim>) => setRows(rows.map((row, i) => (i === index ? { ...row, ...change } : row)));

  function submit(event: FormEvent) {
    event.preventDefault();
    void run(
      "mappings",
      () => api(`/applications/${appId}`, { method: "PATCH", body: { claimMappings: rows } }),
      () => toast({ title: m.saved, description: m.savedHint(appName) }),
      "inline",
    );
  }

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-6">
      {rows.length > 0 ? (
        <SimpleTable
          caption={k.title}
          head={[k.name, t.common.value, t.common.actions]}
          rows={rows.map((row, i) => [
            <Input
              key="n"
              size="sm"
              mono
              className="min-w-48"
              aria-label={m.nameOf(k.noun, i + 1)}
              placeholder={PLACEHOLDER[kind]}
              autoComplete="off"
              value={row.name}
              onChange={(event) => update(i, { name: event.target.value })}
            />,
            <Select
              key="s"
              size="sm"
              className="min-w-48"
              aria-label={m.valueOf(k.noun, i + 1)}
              value={row.source}
              onChange={(event) => update(i, { source: event.target.value })}
            >
              <optgroup label={m.profileGroup}>
                {Object.entries(m.profile).map(([source, label]) => (
                  <option key={source} value={source}>
                    {label}
                  </option>
                ))}
              </optgroup>
              {attributes.length > 0 ? (
                <optgroup label={m.customAttributes}>
                  {attributes.map((a) => (
                    <option key={a.id} value={`attribute.${a.name}`}>
                      {a.label}
                    </option>
                  ))}
                </optgroup>
              ) : null}
              <option value="none">{m.notSent}</option>
            </Select>,
            <Button
              key="r"
              type="button"
              size="icon-sm"
              variant="quiet"
              aria-label={m.removeEntry(row.name || `${k.noun} ${i + 1}`)}
              onClick={() => setRows(rows.filter((_, j) => j !== i))}
            >
              <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
            </Button>,
          ])}
        />
      ) : (
        <p className="rounded-lg border border-border bg-surface px-6 py-4 text-body-sm text-fg-3">
          {k.empty(appName)}
        </p>
      )}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <Button
          type="button"
          size="sm"
          variant="secondary"
          className="self-start"
          onClick={() => setRows([...rows, { name: "", source: attributes[0] ? `attribute.${attributes[0].name}` : "user.id" }])}
        >
          <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
          {m.add}
        </Button>
        <div className="flex flex-col items-start gap-2 sm:items-end">
          <ActionError message={errorFor("mappings")} />
          <Button type="submit" variant="primary" loading={pending === "mappings"}>
            {m.save}
          </Button>
        </div>
      </div>
    </form>
  );
}
