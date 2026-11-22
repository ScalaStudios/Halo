"use client";

import { useState, type FormEvent } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input, Select } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import { useMessages } from "@/lib/i18n/client";
import { applicationsMessages } from "@/lib/i18n/console/applications";
import type { ApplicationHook } from "@/lib/types";

export function HookForm({ appId, appName, hook }: { appId: string; appName: string; hook: ApplicationHook | null }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [url, setUrl] = useState("");
  const [secret, setSecret] = useState("");
  const [claims, setClaims] = useState(hook?.claims.join(", ") ?? "");
  const [failOpen, setFailOpen] = useState(hook?.failOpen ?? false);
  const [errors, setErrors] = useState<{ url?: string; secret?: string }>({});
  const [removing, setRemoving] = useState(false);
  const t = useMessages(applicationsMessages);
  const h = t.hook;

  function submit(event: FormEvent) {
    event.preventDefault();
    const next = {
      url: (hook && !url.trim()) || /^https:\/\/\S+$/.test(url.trim()) ? undefined : h.urlInvalid,
      secret: (hook && !secret) || (secret.trim().length >= 32 && secret.trim().length <= 256) ? undefined : h.secretInvalid,
    };
    setErrors(next);
    if (next.url || next.secret) return;
    const body = { url: url.trim(), secret: secret.trim(), claims: claims.split(/[\s,]+/).filter(Boolean), failOpen };
    void run(
      "save",
      () => api<ApplicationHook>(`/applications/${appId}/hook`, { method: "PUT", body }),
      () => {
        setUrl("");
        setSecret("");
        toast({ title: hook ? h.saved : h.added, description: h.savedHint(appName) });
      },
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader
        title={h.title}
        description={h.description(appName)}
      />
      <form onSubmit={submit} noValidate>
        <CardBody className="grid grid-cols-1 gap-6 md:grid-cols-2">
          <Field
            label={h.url}
            error={errors.url}
            hint={hook ? h.urlKeep(hook.host) : h.urlHint}
          >
            {(props) => (
              <Input
                {...props}
                mono
                type="url"
                autoComplete="off"
                spellCheck={false}
                value={url}
                placeholder={hook ? t.common.unchanged : "https://checks.example.com/halo"}
                onChange={(event) => setUrl(event.target.value)}
              />
            )}
          </Field>
          <Field
            label={h.secret}
            error={errors.secret}
            hint={hook ? h.secretKeep : h.secretHint}
          >
            {(props) => (
              <Input
                {...props}
                mono
                type="password"
                autoComplete="new-password"
                spellCheck={false}
                value={secret}
                placeholder={hook ? t.common.unchanged : undefined}
                onChange={(event) => setSecret(event.target.value)}
              />
            )}
          </Field>
          <Field label={h.claims} hint={h.claimsHint}>
            {(props) => (
              <Input {...props} mono autoComplete="off" spellCheck={false} value={claims} placeholder="department, cost_center" onChange={(event) => setClaims(event.target.value)} />
            )}
          </Field>
          <Field label={h.failure} hint={h.failureHint}>
            {(props) => (
              <Select {...props} value={failOpen ? "open" : "closed"} onChange={(event) => setFailOpen(event.target.value === "open")}>
                <option value="closed">{h.block}</option>
                <option value="open">{h.allow}</option>
              </Select>
            )}
          </Field>
        </CardBody>
        <div className="flex flex-wrap items-center justify-end gap-4 border-t border-border px-6 py-4">
          <ActionError message={errorFor("save")} />
          {hook ? (
            <Button variant="secondary" onClick={() => setRemoving(true)}>
              {h.remove}
            </Button>
          ) : null}
          <Button type="submit" variant="primary" loading={pending === "save"}>
            {hook ? h.save : h.add}
          </Button>
        </div>
      </form>
      <Dialog
        open={removing}
        onOpenChange={(next) => {
          clearError();
          setRemoving(next);
        }}
        error={errorFor("remove")}
        title={h.removeTitle}
        description={h.removeHint(hook?.host, appName)}
        footer={
          <>
            <Button variant="secondary" onClick={() => setRemoving(false)}>
              {t.common.cancel}
            </Button>
            <Button
              variant="destructive"
              loading={pending === "remove"}
              onClick={() =>
                void run(
                  "remove",
                  () => api(`/applications/${appId}/hook`, { method: "DELETE" }),
                  () => {
                    setRemoving(false);
                    setClaims("");
                    setFailOpen(false);
                    toast({ title: h.removed, description: t.common.audit });
                  },
                  "inline",
                )
              }
            >
              {h.remove}
            </Button>
          </>
        }
      />
    </Card>
  );
}
