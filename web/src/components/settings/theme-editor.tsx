"use client";

import { CircleUserRound, RotateCcw, TriangleAlert } from "lucide-react";
import { useId, useState, type CSSProperties } from "react";
import { HaloLogo } from "@/components/halo-mark";
import { ActionError } from "@/components/ui/action-error";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Field, Input } from "@/components/ui/input";
import { METHOD } from "@/lib/labels";
import type { OrganizationProfile, Theme } from "@/lib/settings-types";
import { HALO_THEME, contrast, resolveTheme, themeVariables } from "@/lib/theme";
import { ReadOnlyNote, useSaveSettings } from "./shared";

const COLOURS: { key: keyof Theme; label: string; hint: string }[] = [
  { key: "accent", label: "Accent", hint: "Buttons and highlights." },
  { key: "background", label: "Background", hint: "The page behind everything." },
  { key: "surface", label: "Surface", hint: "Cards and panels." },
  { key: "border", label: "Border", hint: "Outlines and dividers." },
  { key: "text", label: "Text", hint: "Headings and body text." },
  { key: "link", label: "Link", hint: "Links within text." },
];

const KEYS = COLOURS.map((c) => c.key);
const DEFAULTS = Object.fromEntries(KEYS.map((key) => [key, ""])) as Theme;

function hex(value: string) {
  const normalized = value.trim().toLowerCase().replace(/^#?/, "#");
  return /^#[0-9a-f]{6}$/.test(normalized) ? normalized : null;
}

function stored(key: keyof Theme, value: string) {
  const parsed = hex(value);
  return parsed === HALO_THEME[key] ? "" : (parsed ?? value.trim());
}

export function ThemeEditor({ org, saved, canEdit }: { org: OrganizationProfile; saved: Theme; canEdit: boolean }) {
  const { saving, failure, save } = useSaveSettings();
  const warningId = useId();
  const [theme, setTheme] = useState(saved);
  const [errors, setErrors] = useState<Partial<Record<keyof Theme, string>>>({});
  const preview = Object.fromEntries(KEYS.map((key) => [key, hex(theme[key]) ?? ""])) as Theme;
  const colours = resolveTheme(preview);
  const variables = themeVariables(preview, true);
  const dirty = KEYS.some((key) => stored(key, theme[key]) !== saved[key]);
  const pages = [variables["--color-canvas"], variables["--color-surface"]];
  const checks: [string, string, string[]][] = [
    ["text", variables["--color-fg"], pages],
    ["secondary text", variables["--color-fg-3"], pages],
    ["links", variables["--color-link"], pages],
    ["button labels", variables["--color-on-ember"], [variables["--color-ember"]]],
  ];
  const problems = checks
    .map(([label, foreground, backgrounds]) => ({ label, ratio: Math.min(...backgrounds.map((background) => contrast(foreground, background))) }))
    .filter(({ ratio }) => ratio < 4.5);

  function submit() {
    const next = Object.fromEntries(KEYS.map((key) => [key, stored(key, theme[key])])) as Theme;
    const invalid = Object.fromEntries(KEYS.filter((key) => next[key] && !hex(next[key])).map((key) => [key, "Enter a six-digit hex colour, such as #2563eb."]));
    setErrors(invalid);
    if (Object.keys(invalid).length) return;
    void save({ theme: next }, "Colours saved", "The sign-in, setup and account pages use them within 30 seconds.");
  }

  return (
    <Card>
      <CardHeader title="Colours" description="Used on the sign-in, setup and account pages. Hover, pressed and secondary shades are mixed from them. Leave a colour empty to keep Halo's." />
      <CardBody>
        <form
          noValidate
          className="flex flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          <div className="grid grid-cols-1 gap-8 lg:grid-cols-[minmax(0,1fr)_340px]">
            <div className="grid content-start gap-6 sm:grid-cols-2">
              {COLOURS.map(({ key, label, hint }) => (
                <Field key={key} label={label} hint={hint} error={errors[key]}>
                  {(props) => (
                    <div className="flex gap-2">
                      <input
                        type="color"
                        aria-label={`Choose the ${label.toLowerCase()} colour`}
                        disabled={!canEdit}
                        value={colours[key]}
                        onChange={(event) => setTheme((current) => ({ ...current, [key]: event.target.value }))}
                        className="h-10 w-12 shrink-0 cursor-pointer rounded-md border border-border-strong bg-sunken p-1 transition-[border-color] duration-fast ease-brand hover:border-n500 disabled:cursor-default disabled:opacity-45 [&::-moz-color-swatch]:rounded-sm [&::-moz-color-swatch]:border-0 [&::-webkit-color-swatch]:rounded-sm [&::-webkit-color-swatch]:border-0 [&::-webkit-color-swatch-wrapper]:p-0"
                      />
                      <Input
                        {...props}
                        mono
                        maxLength={7}
                        spellCheck={false}
                        autoComplete="off"
                        disabled={!canEdit}
                        value={theme[key]}
                        placeholder={HALO_THEME[key]}
                        onChange={(event) => setTheme((current) => ({ ...current, [key]: event.target.value }))}
                      />
                    </div>
                  )}
                </Field>
              ))}
            </div>
            <figure className="flex flex-col gap-2">
              <div
                inert
                style={variables as CSSProperties}
                className="relative isolate overflow-hidden rounded-lg border border-border bg-canvas px-6 pt-12 pb-8"
              >
                <div aria-hidden="true" className="pointer-events-none absolute inset-x-0 top-0 -z-10 h-48 bg-hearth" />
                <div className="flex flex-col gap-8">
                  <div className="flex flex-col items-center gap-4 text-center">
                    {org.logoUrl ? (
                      <>
                        <img src={org.logoUrl} alt="" className="h-12 w-auto max-w-[240px] object-contain" />
                        <span className="text-h4 text-fg">{org.name}</span>
                      </>
                    ) : (
                      <HaloLogo size="lg" />
                    )}
                    {org.signInMessage ? <p className="line-clamp-3 text-body-sm whitespace-pre-line text-fg-2">{org.signInMessage}</p> : null}
                  </div>
                  <div className="flex flex-col gap-4">
                    <p className="text-center text-h3 text-fg">Choose how to sign in</p>
                    <div className="flex items-center justify-between gap-3 rounded-md border border-border bg-surface py-1 pr-3 pl-4">
                      <span className="flex min-w-0 items-center gap-3">
                        <CircleUserRound aria-hidden="true" size={20} strokeWidth={1.75} className="shrink-0 text-fg-3" />
                        <span className="truncate text-body-sm text-fg">ada@example.com</span>
                      </span>
                      <span className="inline-flex min-h-12 shrink-0 items-center px-1 text-body-sm text-link underline underline-offset-3">Not you?</span>
                    </div>
                  </div>
                  <div className="flex flex-col gap-2">
                    {(["passkey", "totp"] as const).map((kind, index) => {
                      const Icon = METHOD[kind].icon;
                      return (
                        <Button key={kind} variant={index === 0 ? "primary" : "secondary"} size="lg" className="w-full">
                          <Icon aria-hidden="true" size={20} strokeWidth={1.75} className={index === 0 ? undefined : "text-fg-3"} />
                          {METHOD[kind].label}
                        </Button>
                      );
                    })}
                  </div>
                </div>
              </div>
              <figcaption className="text-caption text-fg-3">Preview of the sign-in page. Changes apply when you save.</figcaption>
            </figure>
          </div>
          {problems.length ? (
            <div id={warningId} role="note" className="flex gap-3 rounded-md border border-warning/20 bg-warning-container p-4">
              <TriangleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0 text-warning" />
              <p className="tnum text-body-sm text-fg-2">
                Hard to read: {problems.map(({ label, ratio }) => `${label} ${ratio.toFixed(1)}:1`).join(", ")}. WCAG AA asks for at least 4.5:1, so pick colours further apart.
              </p>
            </div>
          ) : null}
          {canEdit ? (
            <div className="flex flex-wrap gap-3">
              <Button type="submit" variant="secondary" loading={saving} disabled={!dirty} aria-describedby={problems.length ? warningId : undefined}>
                Save colours
              </Button>
              <Button
                variant="quiet"
                disabled={KEYS.every((key) => !theme[key])}
                onClick={() => {
                  setTheme(DEFAULTS);
                  setErrors({});
                }}
              >
                <RotateCcw aria-hidden="true" size={16} strokeWidth={1.75} />
                Reset to Halo defaults
              </Button>
              <ActionError message={failure} className="basis-full" />
            </div>
          ) : (
            <ReadOnlyNote>Only a global administrator can change the colours.</ReadOnlyNote>
          )}
        </form>
      </CardBody>
    </Card>
  );
}
