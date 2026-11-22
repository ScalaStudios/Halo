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
import { useMessages } from "@/lib/i18n/client";
import { settingsMessages } from "@/lib/i18n/console/settings";
import type { ImportChange, ImportReport } from "@/lib/settings-types";

type Chosen = { name: string; configuration: Record<string, unknown> };

type Messages = (typeof settingsMessages)["en"]["import"];

const SECTIONS = settingsMessages.en.import.sections;

const TONES: Record<ImportChange["action"], Tone> = { create: "success", change: "info", remove: "danger" };

async function read(file: File, notExport: Messages["notExport"]): Promise<Chosen> {
  const parsed: unknown = await file
    .text()
    .then(JSON.parse)
    .catch(() => null);
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new ApiError(422, "ERR_INVALID", notExport(file.name));
  }
  const configuration = Object.fromEntries(Object.entries(parsed).filter(([key]) => key in SECTIONS));
  return { name: file.name, configuration };
}

function counts(report: ImportReport, t: Messages) {
  return (["create", "change", "remove"] as const)
    .map((action) => [report.changes.filter((c) => c.action === action).length, t.counts[action]] as const)
    .filter(([n]) => n)
    .map(([n, label]) => label(n));
}

export function ImportConfiguration({ canImport }: { canImport: boolean }) {
  const input = useRef<HTMLInputElement>(null);
  const [chosen, setChosen] = useState<Chosen | null>(null);
  const [remove, setRemove] = useState(false);
  const [report, setReport] = useState<ImportReport | null>(null);
  const { pending, run, errorFor, clearError, toast } = useMutation();
  const { import: t, common } = useMessages(settingsMessages);

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
        toast({ title: t.imported, description: t.importedHint(done.changes.length, chosen.name) });
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
            {t.choose}
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
          if (file) check(read(file, t.notExport), remove);
        }}
      />
      <Dialog
        open={report !== null}
        onOpenChange={(open) => (open ? undefined : close())}
        width="wide"
        error={errorFor("preview") ?? errorFor("import")}
        title={t.checkTitle}
        description={t.checkDescription(chosen?.name ?? t.theFile)}
        footer={
          <>
            <Button variant="secondary" onClick={close}>
              {common.cancel}
            </Button>
            <Button variant="primary" loading={pending === "import"} disabled={!report?.changes.length || refused.length > 0 || pending === "preview"} onClick={apply}>
              {report?.changes.length ? t.importCount(report.changes.length) : t.nothing}
            </Button>
          </>
        }
      >
        {report ? (
          <div className="flex animate-enter flex-col gap-6">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <p aria-live="polite" className="text-body-sm text-fg-2">
                {refused.length
                  ? t.refused(refused.length, chosen?.name ?? t.theFile)
                  : report.changes.length
                    ? `${joinList(counts(report, t), common.and)}.`
                    : t.matches}
              </p>
              <Button size="sm" variant="secondary" loading={pending === "preview"} disabled={pending === "import"} onClick={() => input.current?.click()}>
                {t.another}
              </Button>
            </div>
            {report.secrets.length ? (
              <SecretWarning>
                {t.secrets(
                  joinList(report.secrets.map((s) => s.label), common.and),
                  report.secrets.some((s) => s.fields?.includes("client secret")),
                  report.secrets.length,
                )}
              </SecretWarning>
            ) : null}
            {report.changes.length ? (
              <SimpleTable
                caption={t.changesFrom(chosen?.name ?? t.theFile)}
                head={[t.change, t.object]}
                rows={[...report.changes]
                  .sort((a, b) => Number(!a.error) - Number(!b.error))
                  .map((change) => [
                    <Badge key="action" tone={TONES[change.action]}>
                      {t.actions[change.action]}
                    </Badge>,
                    <span key="object" className="flex flex-col">
                      <span className="font-medium break-all text-fg">{change.label}</span>
                      <span className="text-caption text-fg-3">
                        {t.sections[change.section as keyof typeof SECTIONS] ?? change.section}
                        {change.fields?.length ? ` · ${change.fields.join(", ")}` : ""}
                      </span>
                      {change.error ? (
                        <span lang="en" className="mt-1 flex items-start gap-2 text-danger">
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
                <span className="text-body-sm text-fg">{t.remove}</span>
                <span className="text-caption text-fg-3">{t.removeHint}</span>
              </span>
            </label>
          </div>
        ) : null}
      </Dialog>
    </>
  );
}
