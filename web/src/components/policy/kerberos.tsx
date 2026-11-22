"use client";

import { Plus, Trash2, Upload, X } from "lucide-react";
import { useRef, useState } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { DescriptionList } from "@/components/ui/description-list";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { formatDate } from "@/lib/format";
import { useLocale } from "@/lib/i18n/client";
import { policyMessages } from "@/lib/i18n/console/policy";
import type { KerberosRealm, KerberosSource, NetworkZone } from "@/lib/policy-types";
import { CheckList } from "./policy-form";

async function base64(file: File) {
  let binary = "";
  for (const byte of new Uint8Array(await file.arrayBuffer())) binary += String.fromCharCode(byte);
  return btoa(binary);
}

export function KerberosSettings({ source, zones, host }: { source: KerberosSource | null; zones: NetworkZone[]; host: string }) {
  const locale = useLocale();
  const copy = policyMessages[locale];
  const t = copy.kerberos;
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const file = useRef<HTMLInputElement>(null);
  const [spn, setSpn] = useState(source?.servicePrincipal ?? `HTTP/${host}@`);
  const [realms, setRealms] = useState<KerberosRealm[]>(source?.realms ?? [{ realm: "", domain: "" }]);
  const [zoneIds, setZoneIds] = useState(source?.zoneIds ?? []);
  const [keytab, setKeytab] = useState<{ name: string; data: string } | null>(null);
  const [errors, setErrors] = useState<{ spn?: string; realms?: string; keytab?: string }>({});
  const [removing, setRemoving] = useState(false);

  function update(index: number, patch: Partial<KerberosRealm>) {
    setRealms((current) => current.map((r, i) => (i === index ? { ...r, ...patch } : r)));
  }

  function save() {
    const next = {
      spn: /^HTTP\/[^\s/@]+@[^\s@]+$/i.test(spn.trim()) ? undefined : t.spnInvalid(host),
      realms: realms.length > 0 && realms.every((r) => r.realm.trim() && r.domain.trim()) ? undefined : t.realmsRequired,
      keytab: source || keytab ? undefined : t.keytabRequired,
    };
    setErrors(next);
    if (next.spn || next.realms || next.keytab) return;
    void run(
      "save",
      () => api<KerberosSource>("/kerberos", { method: "PUT", body: { servicePrincipal: spn.trim(), realms, zoneIds, ...(keytab ? { keytab: keytab.data } : {}) } }),
      (saved) => {
        setKeytab(null);
        setSpn(saved.servicePrincipal);
        setRealms(saved.realms);
        toast({ title: source ? t.saved : t.setUp, description: t.savedDescription });
      },
      "inline",
    );
  }

  function remove() {
    void run("remove", () => api("/kerberos", { method: "DELETE" }), () => {
      setRemoving(false);
      toast({ title: t.removed, description: t.removedDescription });
    }, "inline");
  }

  return (
    <Card>
      <CardHeader
        title={t.title}
        description={t.description(host)}
        actions={
          source ? (
            <Button size="icon" variant="secondary" aria-label={t.removeLabel} onClick={() => setRemoving(true)}>
              <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
            </Button>
          ) : null
        }
      />
      <CardBody>
        <form
          noValidate
          className="flex max-w-xl flex-col gap-6"
          onSubmit={(event) => {
            event.preventDefault();
            save();
          }}
        >
          <Field label={t.spn} hint={t.spnHint} error={errors.spn}>
            {(props) => <Input {...props} mono autoComplete="off" spellCheck={false} value={spn} onChange={(event) => setSpn(event.target.value)} placeholder={`HTTP/${host}@CORP.EXAMPLE.COM`} />}
          </Field>
          <fieldset className="flex flex-col gap-3" aria-describedby={errors.realms ? "realms-error" : undefined}>
            <legend className="mb-1 text-label text-fg">{t.realms}</legend>
            <p className="text-caption text-fg-3">{t.realmsHint}</p>
            {realms.map((r, index) => (
              <div key={index} className="flex items-start gap-2">
                <Input mono aria-label={t.realm(index + 1)} autoComplete="off" spellCheck={false} value={r.realm} onChange={(event) => update(index, { realm: event.target.value })} placeholder="CORP.EXAMPLE.COM" />
                <Input mono aria-label={t.domain(index + 1)} autoComplete="off" spellCheck={false} value={r.domain} onChange={(event) => update(index, { domain: event.target.value })} placeholder="example.com" />
                <Button size="icon" variant="quiet" aria-label={t.removeRealm(r.realm || index + 1)} disabled={realms.length === 1} onClick={() => setRealms((current) => current.filter((_, i) => i !== index))}>
                  <X aria-hidden="true" size={16} strokeWidth={1.75} />
                </Button>
              </div>
            ))}
            {errors.realms ? (
              <p id="realms-error" className="text-caption text-danger">
                {errors.realms}
              </p>
            ) : null}
            <Button size="sm" variant="secondary" className="self-start" onClick={() => setRealms((current) => [...current, { realm: "", domain: "" }])}>
              <Plus aria-hidden="true" size={16} strokeWidth={1.75} />
              {t.addRealm}
            </Button>
          </fieldset>
          <CheckList
            legend={t.zones}
            options={zones.filter((z) => z.kind === "trusted").map((z) => ({ id: z.id, name: z.name, detail: z.cidrs.join(", ") }))}
            selected={zoneIds}
            onToggle={(id, on) => setZoneIds((current) => (on ? [...current, id] : current.filter((z) => z !== id)))}
            empty={t.noZones}
            scroll={false}
          />
          <Field label={t.keytab} hint={source ? t.keytabKeep : t.keytabHint} error={errors.keytab}>
            {(props) => (
              <div className="flex flex-wrap items-center gap-3">
                <input
                  ref={file}
                  type="file"
                  accept=".keytab,application/octet-stream"
                  hidden
                  onChange={(event) => {
                    const chosen = event.target.files?.[0];
                    event.target.value = "";
                    if (chosen) void base64(chosen).then((data) => setKeytab({ name: chosen.name, data }));
                  }}
                />
                <Button {...props} size="sm" variant="secondary" onClick={() => file.current?.click()}>
                  <Upload aria-hidden="true" size={16} strokeWidth={1.75} />
                  {source ? t.replace : t.choose}
                </Button>
                {keytab ? <span className="font-mono text-code-sm break-all text-fg-2">{keytab.name}</span> : null}
              </div>
            )}
          </Field>
          {source?.keys.length ? (
            <DescriptionList
              columns={2}
              items={[
                { label: t.savedKeys, value: source.keys.map((k) => t.key(k.type, k.version)).join("; ") },
                { label: t.exported, value: formatDate(source.keys[0]!.createdAt, locale) },
              ]}
            />
          ) : null}
          <div className="flex flex-wrap items-center gap-4">
            <Button type="submit" variant="primary" loading={pending === "save"}>
              {source ? t.save : t.setUpButton}
            </Button>
            <ActionError message={errorFor("save")} />
          </div>
        </form>
      </CardBody>
      <Dialog
        open={removing}
        onOpenChange={(open) => {
          clearError();
          setRemoving(open);
        }}
        error={errorFor("remove")}
        title={t.removeTitle}
        description={t.removeDescription}
        footer={
          <>
            <Button variant="secondary" onClick={() => setRemoving(false)}>
              {copy.cancel}
            </Button>
            <Button variant="destructive" loading={pending === "remove"} onClick={remove}>
              {copy.remove}
            </Button>
          </>
        }
      />
    </Card>
  );
}
