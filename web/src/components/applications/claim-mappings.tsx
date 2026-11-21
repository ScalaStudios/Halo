"use client";

import { Plus, Trash2 } from "lucide-react";
import { useState, type FormEvent } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Button } from "@/components/ui/button";
import { Input, Select } from "@/components/ui/input";
import { SimpleTable } from "@/components/ui/simple-table";
import { api } from "@/lib/api/client";
import type { Attribute, Claim } from "@/lib/types";

const PROFILE = [
  ["user.id", "User ID"],
  ["user.email", "Email"],
  ["user.emailVerified", "Email verified"],
  ["user.name", "Name"],
  ["user.givenName", "Given name"],
  ["user.familyName", "Family name"],
  ["user.title", "Title"],
  ["user.department", "Department"],
  ["user.location", "Location"],
  ["user.groups", "Group names"],
] as const;

export function ClaimMappingsForm({ appId, appName, saml, mappings, attributes }: { appId: string; appName: string; saml: boolean; mappings: Claim[]; attributes: Attribute[] }) {
  const [rows, setRows] = useState<Claim[]>(mappings);
  const { pending, run, errorFor, toast } = useMutation();
  const noun = saml ? "attribute" : "claim";
  const update = (index: number, change: Partial<Claim>) => setRows(rows.map((row, i) => (i === index ? { ...row, ...change } : row)));

  function submit(event: FormEvent) {
    event.preventDefault();
    void run(
      "mappings",
      () => api(`/applications/${appId}`, { method: "PATCH", body: { claimMappings: rows } }),
      () => toast({ title: "Mappings saved", description: `${appName} receives them on its next sign-in. Recorded in the audit log.` }),
      "inline",
    );
  }

  return (
    <form onSubmit={submit} noValidate className="flex flex-col gap-6">
      {rows.length > 0 ? (
        <SimpleTable
          caption={saml ? "Attribute mappings" : "Claim mappings"}
          head={[saml ? "SAML attribute name" : "Claim name", "Value", "Actions"]}
          rows={rows.map((row, i) => [
            <Input
              key="n"
              size="sm"
              mono
              className="min-w-48"
              aria-label={`Name of ${noun} ${i + 1}`}
              placeholder={saml ? "employeeNumber" : "employee_id"}
              autoComplete="off"
              value={row.name}
              onChange={(event) => update(i, { name: event.target.value })}
            />,
            <Select
              key="s"
              size="sm"
              className="min-w-48"
              aria-label={`Value of ${noun} ${i + 1}`}
              value={row.source}
              onChange={(event) => update(i, { source: event.target.value })}
            >
              <optgroup label="Profile">
                {PROFILE.map(([source, label]) => (
                  <option key={source} value={source}>
                    {label}
                  </option>
                ))}
              </optgroup>
              {attributes.length > 0 ? (
                <optgroup label="Custom attributes">
                  {attributes.map((a) => (
                    <option key={a.id} value={`attribute.${a.name}`}>
                      {a.label}
                    </option>
                  ))}
                </optgroup>
              ) : null}
              <option value="none">Not sent</option>
            </Select>,
            <Button
              key="r"
              type="button"
              size="icon-sm"
              variant="quiet"
              aria-label={`Remove ${row.name || `${noun} ${i + 1}`}`}
              onClick={() => setRows(rows.filter((_, j) => j !== i))}
            >
              <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
            </Button>,
          ])}
        />
      ) : (
        <p className="rounded-lg border border-border bg-surface px-6 py-4 text-body-sm text-fg-3">
          No mappings. {appName} receives the {noun}s above.
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
          Add mapping
        </Button>
        <div className="flex flex-col items-start gap-2 sm:items-end">
          <ActionError message={errorFor("mappings")} />
          <Button type="submit" variant="primary" loading={pending === "mappings"}>
            Save mappings
          </Button>
        </div>
      </div>
    </form>
  );
}
