"use client";

import { CircleAlert, Upload } from "lucide-react";
import { useRef, useState } from "react";
import { SecretWarning } from "@/components/applications/shared";
import { useMutation } from "@/components/console/use-mutation";
import { joinList } from "@/components/policy/shared";
import { ActionError } from "@/components/ui/action-error";
import { Badge, type Tone } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog } from "@/components/ui/dialog";
import { Checkbox } from "@/components/ui/input";
import { SimpleTable } from "@/components/ui/simple-table";
import { api, ApiError } from "@/lib/api/client";
import { pluralize } from "@/lib/format";
import type { ImportChange, ImportReport } from "@/lib/settings-types";

type Chosen = { name: string; configuration: Record<string, unknown> };

const SECTIONS: Record<string, string> = {
  settings: "Settings",
  attributes: "Attribute",
  groups: "Group",
  memberships: "Group member",
  roles: "Role",
  eligibleRoles: "Eligible role",
  terms: "Terms of use",
  applications: "Application",
  policies: "Policy",
  accessPackages: "Access package",
};

const ACTIONS: Record<ImportChange["action"], { label: string; tone: Tone }> = {
  create: { label: "Create", tone: "success" },
  change: { label: "Change", tone: "info" },
  remove: { label: "Remove", tone: "danger" },
};

async function read(file: File): Promise<Chosen> {
  const parsed: unknown = await file
    .text()
    .then(JSON.parse)
    .catch(() => null);
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new ApiError(422, "ERR_INVALID", `${file.name} is not a Halo export. Choose the JSON file from Download export.`);
  }
  const configuration = Object.fromEntries(Object.entries(parsed).filter(([key]) => key in SECTIONS));
  return { name: file.name, configuration };
}

function counts(report: ImportReport) {
  const total = (action: ImportChange["action"]) => report.changes.filter((c) => c.action === action).length;
  return [
    [total("create"), "to create"],
    [total("change"), "to change"],
    [total("remove"), "to remove"],
  ]
    .filter(([n]) => n)
    .map(([n, label]) => `${n} ${label}`);
}

export function ImportConfiguration({ canImport }: { canImport: boolean }) {
  const input = useRef<HTMLInputElement>(null);
  const [chosen, setChosen] = useState<Chosen | null>(null);
  const [remove, setRemove] = useState(false);
  const [report, setReport] = useState<ImportReport | null>(null);
  const { pending, run, errorFor, clearError, toast } = useMutation();

  function check(next: Chosen | Promise<Chosen>, removing: boolean) {
    void run(
      "preview",
      async () => {
        const file = await next;
        return { file, report: await api<ImportReport>("/import", { body: { configuration: file.configuration, remove: removing, dryRun: true } }) };
      },
      (result) => {
        setChosen(result.file);
        setRemove(removing);
        setReport(result.report);
      },
      "inline",
    );
  }

  function close() {
    setChosen(null);
    setReport(null);
    setRemove(false);
    clearError();
  }

  function apply() {
    if (!chosen || !report) return;
    void run(
      "import",
      () => api<ImportReport>("/import", { body: { configuration: chosen.configuration, remove } }),
      (done) => {
        close();
        toast({ title: "Configuration imported", description: `Halo made ${pluralize(done.changes.length, "change")} from ${chosen.name}. Each one is in the audit log.` });
      },
      "inline",
    );
  }

  const refused = report?.changes.filter((c) => c.error) ?? [];
  const busy = pending === "preview" || pending === "import";

  return (
    <>
      <div className="flex flex-col gap-3">
        <div>
          <Button variant="secondary" loading={pending === "preview" && !report} disabled={!canImport || busy} onClick={() => input.current?.click()}>
            <Upload aria-hidden="true" size={16} strokeWidth={1.75} />
            Choose export file
          </Button>
        </div>
        {report ? null : <ActionError message={errorFor("preview")} />}
      </div>
      <input
        ref={input}
        type="file"
        accept=".json,application/json"
        hidden
        onChange={(event) => {
          const file = event.target.files?.[0];
          event.target.value = "";
          if (file) check(read(file), remove);
        }}
      />
      <Dialog
        open={report !== null}
        onOpenChange={(open) => (open ? undefined : close())}
        width="wide"
        error={errorFor("preview") ?? errorFor("import")}
        title="Check the import"
        description={`Nothing changes until you confirm. Halo checked every object in ${chosen?.name ?? "the file"} with the same rules as the console.`}
        footer={
          <>
            <Button variant="secondary" onClick={close}>
              Cancel
            </Button>
            <Button variant="primary" loading={pending === "import"} disabled={!report?.changes.length || refused.length > 0 || pending === "preview"} onClick={apply}>
              {report?.changes.length ? `Import ${pluralize(report.changes.length, "change")}` : "Nothing to import"}
            </Button>
          </>
        }
      >
        {report ? (
          <div className="flex animate-enter flex-col gap-6">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <p aria-live="polite" className="text-body-sm text-fg-2">
                {refused.length
                  ? `${pluralize(refused.length, "object")} can't be imported, so nothing will be. Fix ${refused.length === 1 ? "it" : "them"} in ${chosen?.name} and choose it again.`
                  : report.changes.length
                    ? `${joinList(counts(report), "and")}.`
                    : "Nothing to change. This installation already matches the file."}
              </p>
              <Button size="sm" variant="secondary" loading={pending === "preview"} disabled={pending === "import"} onClick={() => input.current?.click()}>
                Choose another file
              </Button>
            </div>
            {report.secrets.length ? (
              <SecretWarning>
                {joinList(report.secrets.map((s) => s.label), "and")} will get a new client ID{report.secrets.some((s) => s.fields?.includes("client secret")) ? " and client secret" : ""}. After
                the import, update {report.secrets.length === 1 ? "it" : "them"} wherever the application is configured, and create a client secret on its page.
              </SecretWarning>
            ) : null}
            {report.changes.length ? (
              <SimpleTable
                caption={`Changes from ${chosen?.name ?? "the file"}`}
                head={["Change", "Object"]}
                rows={[...report.changes]
                  .sort((a, b) => Number(!a.error) - Number(!b.error))
                  .map((change) => [
                    <Badge key="action" tone={ACTIONS[change.action].tone}>
                      {ACTIONS[change.action].label}
                    </Badge>,
                    <span key="object" className="flex flex-col">
                      <span className="font-medium break-all text-fg">{change.label}</span>
                      <span className="text-caption text-fg-3">
                        {SECTIONS[change.section] ?? change.section}
                        {change.fields?.length ? ` · ${change.fields.join(", ")}` : ""}
                      </span>
                      {change.error ? (
                        <span className="mt-1 flex items-start gap-2 text-danger">
                          <CircleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0" />
                          {change.error.message}
                        </span>
                      ) : null}
                    </span>,
                  ])}
              />
            ) : null}
            <label className="flex cursor-pointer items-start gap-3">
              <Checkbox className="mt-0.5" checked={remove} disabled={busy} onChange={(event) => chosen && check(chosen, event.target.checked)} />
              <span className="flex flex-col">
                <span className="text-body-sm text-fg">Remove what isn&apos;t in the file</span>
                <span className="text-caption text-fg-3">
                  Deletes objects of each kind the file includes when the file doesn&apos;t have them, and removes people from the file&apos;s groups.
                </span>
              </span>
            </label>
          </div>
        ) : null}
      </Dialog>
    </>
  );
}
