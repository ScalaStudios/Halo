"use client";

import { Field, Input, Select, Textarea } from "@/components/ui/input";
import { useMessages } from "@/lib/i18n/client";
import { directoryMessages } from "@/lib/i18n/console/directory";
import type { Locale } from "@/lib/i18n/locale";
import type { Attribute, AttributeValue } from "@/lib/types";

export type Drafts = Record<string, string>;

export function draftsOf(attributes: Attribute[], values: Record<string, AttributeValue> | null) {
  return Object.fromEntries(
    attributes.map((a) => {
      const value = values?.[a.name];
      return [a.name, value === undefined ? "" : Array.isArray(value) ? value.join("\n") : String(value)];
    }),
  ) as Drafts;
}

export function parseDrafts(attributes: Attribute[], drafts: Drafts, locale: Locale = "en") {
  const values: Record<string, AttributeValue | null> = {};
  const errors: Record<string, string> = {};
  for (const a of attributes) {
    const draft = drafts[a.name]?.trim() ?? "";
    if (draft === "") values[a.name] = null;
    else if (a.type === "number") {
      if (Number.isFinite(Number(draft))) values[a.name] = Number(draft);
      else errors[a.name] = directoryMessages[locale].attributeFields.number;
    } else if (a.type === "boolean") values[a.name] = draft === "true";
    else if (a.type === "list")
      values[a.name] = draft
        .split("\n")
        .map((line) => line.trim())
        .filter(Boolean);
    else values[a.name] = draft;
  }
  return { values, errors };
}

export function AttributeFields({
  attributes,
  drafts,
  errors,
  onChange,
}: {
  attributes: Attribute[];
  drafts: Drafts;
  errors: Record<string, string>;
  onChange: (drafts: Drafts) => void;
}) {
  const t = useMessages(directoryMessages).attributeFields;
  return attributes.map((a) => {
    const set = (value: string) => onChange({ ...drafts, [a.name]: value });
    return (
      <Field key={a.id} label={a.label} error={errors[a.name]} hint={a.type === "list" ? t.listHint : undefined}>
        {(props) =>
          a.type === "boolean" ? (
            <Select {...props} value={drafts[a.name]} onChange={(event) => set(event.target.value)}>
              <option value="">{t.notSet}</option>
              <option value="true">{t.yes}</option>
              <option value="false">{t.no}</option>
            </Select>
          ) : a.type === "list" ? (
            <Textarea {...props} value={drafts[a.name]} onChange={(event) => set(event.target.value)} />
          ) : (
            <Input
              {...props}
              type={a.type === "date" ? "date" : "text"}
              inputMode={a.type === "number" ? "decimal" : undefined}
              autoComplete="off"
              value={drafts[a.name]}
              onChange={(event) => set(event.target.value)}
            />
          )
        }
      </Field>
    );
  });
}
