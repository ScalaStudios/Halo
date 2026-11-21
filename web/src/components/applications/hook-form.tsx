"use client";

import { useState, type FormEvent } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Button } from "@/components/ui/button";
import { Card, CardBody, CardHeader } from "@/components/ui/card";
import { Dialog } from "@/components/ui/dialog";
import { Field, Input, Select } from "@/components/ui/input";
import { api } from "@/lib/api/client";
import type { ApplicationHook } from "@/lib/types";

export function HookForm({ appId, appName, hook }: { appId: string; appName: string; hook: ApplicationHook | null }) {
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const [url, setUrl] = useState("");
  const [secret, setSecret] = useState("");
  const [claims, setClaims] = useState(hook?.claims.join(", ") ?? "");
  const [failOpen, setFailOpen] = useState(hook?.failOpen ?? false);
  const [errors, setErrors] = useState<{ url?: string; secret?: string }>({});
  const [removing, setRemoving] = useState(false);

  function submit(event: FormEvent) {
    event.preventDefault();
    const next = {
      url: (hook && !url.trim()) || /^https:\/\/\S+$/.test(url.trim()) ? undefined : "Enter the https:// URL of the hook.",
      secret: (hook && !secret) || (secret.trim().length >= 32 && secret.trim().length <= 256) ? undefined : "Enter a signing secret of 32 to 256 characters.",
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
        toast({ title: hook ? "Hook saved" : "Hook added", description: `Halo calls it from the next sign-in to ${appName}. Recorded in the audit log.` });
      },
      "inline",
    );
  }

  return (
    <Card>
      <CardHeader
        title="Sign-in hook"
        description={`Halo calls this endpoint when someone signs in to ${appName} and before it issues each token. It can deny access with a reason shown to the person, or add claims. Every call and its decision appear in the sign-in log.`}
      />
      <form onSubmit={submit} noValidate>
        <CardBody className="grid grid-cols-1 gap-6 md:grid-cols-2">
          <Field
            label="Hook URL"
            error={errors.url}
            hint={hook ? `Leave empty to keep the URL for ${hook.host}. Halo stores it encrypted and never shows it again.` : "Halo stores it encrypted and never shows it again. Private and local addresses are refused."}
          >
            {(props) => (
              <Input
                {...props}
                mono
                type="url"
                autoComplete="off"
                spellCheck={false}
                value={url}
                placeholder={hook ? "Unchanged" : "https://checks.example.com/halo"}
                onChange={(event) => setUrl(event.target.value)}
              />
            )}
          </Field>
          <Field
            label="Signing secret"
            error={errors.secret}
            hint={hook ? "Leave empty to keep the current secret." : "32 to 256 characters. Halo signs each request with it, and the hook signs its answer with it."}
          >
            {(props) => (
              <Input
                {...props}
                mono
                type="password"
                autoComplete="new-password"
                spellCheck={false}
                value={secret}
                placeholder={hook ? "Unchanged" : undefined}
                onChange={(event) => setSecret(event.target.value)}
              />
            )}
          </Field>
          <Field label="Claims the hook can add" hint="Separate names with commas. Halo drops any other claim, and never lets a hook set sub, iss, aud, exp, email, groups, roles or another claim it sets itself.">
            {(props) => (
              <Input {...props} mono autoComplete="off" spellCheck={false} value={claims} placeholder="department, cost_center" onChange={(event) => setClaims(event.target.value)} />
            )}
          </Field>
          <Field label="When the hook fails" hint="A failure is no answer within 3 seconds, a status other than 200, a missing or wrong signature, or an unreadable answer.">
            {(props) => (
              <Select {...props} value={failOpen ? "open" : "closed"} onChange={(event) => setFailOpen(event.target.value === "open")}>
                <option value="closed">Block the sign-in</option>
                <option value="open">Allow the sign-in without added claims</option>
              </Select>
            )}
          </Field>
        </CardBody>
        <div className="flex flex-wrap items-center justify-end gap-4 border-t border-border px-6 py-4">
          <ActionError message={errorFor("save")} />
          {hook ? (
            <Button variant="secondary" onClick={() => setRemoving(true)}>
              Remove hook
            </Button>
          ) : null}
          <Button type="submit" variant="primary" loading={pending === "save"}>
            {hook ? "Save hook" : "Add hook"}
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
        title="Remove the sign-in hook?"
        description={`Halo stops calling ${hook?.host ?? "the hook"}. Sign-ins to ${appName} are no longer checked by it, and tokens lose the claims it added.`}
        footer={
          <>
            <Button variant="secondary" onClick={() => setRemoving(false)}>
              Cancel
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
                    toast({ title: "Hook removed", description: "Recorded in the audit log." });
                  },
                  "inline",
                )
              }
            >
              Remove hook
            </Button>
          </>
        }
      />
    </Card>
  );
}
