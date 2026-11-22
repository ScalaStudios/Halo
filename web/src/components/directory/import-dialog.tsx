"use client";

import { CircleAlert, CircleCheck } from "lucide-react";
import { useRef, useState, type ReactNode } from "react";
import { SecretWarning } from "@/components/applications/shared";
import { useMutation } from "@/components/console/use-mutation";
import { Tag } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/ui/copy-button";
import { Dialog } from "@/components/ui/dialog";
import { Checkbox } from "@/components/ui/input";
import { SimpleTable } from "@/components/ui/simple-table";
import { send } from "@/lib/api/client";
import { useMessages } from "@/lib/i18n/client";
import { directoryMessages } from "@/lib/i18n/console/directory";
import type { SetupLink } from "@/lib/mail-types";

type ImportRow = Partial<SetupLink> & {
  row: number;
  email: string;
  name: string;
  title: string;
  department: string;
  location: string;
  managerEmail: string;
  groups: string[];
  error?: { code: string; message: string };
};

type ImportResult = { dryRun: boolean; ready: number; skipped: number; rows: ImportRow[] };

const COLUMNS = ["email", "name", "title", "department", "location", "manager email", "groups"];

function Person({ row, children }: { row: ImportRow; children?: ReactNode }) {
  const t = useMessages(directoryMessages).import;
  const details = [row.title, row.department, row.location, row.managerEmail && t.reportsTo(row.managerEmail), row.groups.join(", ")].filter(Boolean);
  return (
    <span className="flex flex-col">
      <span className="font-medium text-fg">{row.name || t.noName}</span>
      <span className="text-caption break-all text-fg-3">{row.email || t.noEmail}</span>
      {details.length ? <span className="text-caption text-fg-3">{details.join(" · ")}</span> : null}
      {children}
    </span>
  );
}

export function ImportDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const input = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [notify, setNotify] = useState(true);
  const [result, setResult] = useState<ImportResult | null>(null);
  const { pending, run, errorFor, clearError } = useMutation();
  const m = useMessages(directoryMessages);
  const t = m.import;

  function close() {
    onOpenChange(false);
    setFile(null);
    setNotify(true);
    setResult(null);
    clearError();
  }

  function upload(chosen: File, dryRun: boolean) {
    void run(
      dryRun ? "preview" : "import",
      async () => {
        const res = await send(
          `/users/import?dryRun=${dryRun}&sendInvitations=${notify}`,
          { method: "POST", body: chosen, headers: { "content-type": "text/csv" } },
          t.readFailed,
        );
        return (await res.json()) as ImportResult;
      },
      (next) => {
        setFile(chosen);
        setResult(next);
      },
      "inline",
    );
  }

  const preview = result?.dryRun ? result : null;
  const done = result && !result.dryRun ? result : null;
  const created = done?.rows.filter((row) => !row.error) ?? [];
  const held = created.filter((row) => !row.emailed);

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => (next ? onOpenChange(true) : close())}
      width={result ? "wide" : "default"}
      error={errorFor("preview") ?? errorFor("import")}
      title={done ? t.doneTitle : preview ? t.previewTitle : m.users.importUsers}
      description={done ? t.doneDescription(created.length, file?.name, done.skipped) : preview ? t.previewDescription(file?.name) : t.description}
      footer={
        done ? (
          <Button variant="primary" onClick={close}>
            {m.done}
          </Button>
        ) : (
          <>
            <Button variant="secondary" onClick={close}>
              {m.cancel}
            </Button>
            {preview ? (
              <Button variant="primary" loading={pending === "import"} disabled={preview.ready === 0 || pending === "preview"} onClick={() => file && upload(file, false)}>
                {t.importPeople(preview.ready)}
              </Button>
            ) : (
              <Button variant="primary" loading={pending === "preview"} onClick={() => input.current?.click()}>
                {t.chooseFile}
              </Button>
            )}
          </>
        )
      }
    >
      <input
        ref={input}
        type="file"
        accept=".csv,text/csv"
        hidden
        onChange={(event) => {
          const chosen = event.target.files?.[0];
          event.target.value = "";
          if (chosen) upload(chosen, true);
        }}
      />
      {done ? (
        <div className="flex animate-enter flex-col gap-4">
          {created.length === 0 ? null : held.length ? (
            <SecretWarning>{t.heldLinks(held.length, created.length)}</SecretWarning>
          ) : (
            <p className="text-body-sm text-fg-2">{t.emailedAll}</p>
          )}
          {created.length ? (
            <SimpleTable
              caption={t.createdCaption}
              head={[t.person, m.setupLink]}
              rows={created.map((row) => [
                <Person key="person" row={row} />,
                row.emailed ? <span className="whitespace-nowrap text-fg-3">{t.emailed}</span> : <CopyButton value={row.enrollUrl ?? ""} label={t.copyLink} className="whitespace-nowrap" />,
              ])}
            />
          ) : null}
        </div>
      ) : preview ? (
        <div className="flex animate-enter flex-col gap-6">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p aria-live="polite" className="text-body-sm text-fg-2">
              {t.ready(preview.ready, preview.skipped)}
            </p>
            <Button size="sm" variant="secondary" loading={pending === "preview"} disabled={pending === "import"} onClick={() => input.current?.click()}>
              {t.chooseAnother}
            </Button>
          </div>
          <SimpleTable
            caption={t.rowsIn(file?.name)}
            head={[t.row, t.person]}
            rows={[...preview.rows]
              .sort((a, b) => Number(!a.error) - Number(!b.error))
              .map((row) => [
                <span key="row" className="tnum text-fg-3">
                  {row.row}
                </span>,
                <Person key="person" row={row}>
                  {row.error ? (
                    <span className="mt-1 flex items-start gap-2 text-danger">
                      <CircleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0" />
                      <span lang="en">{row.error.message}</span>
                    </span>
                  ) : (
                    <span className="mt-1 flex items-center gap-2 text-success">
                      <CircleCheck aria-hidden="true" size={16} strokeWidth={1.75} className="shrink-0" />
                      {t.readyRow}
                    </span>
                  )}
                </Person>,
              ])}
          />
          <label className="flex cursor-pointer items-start gap-3">
            <Checkbox className="mt-0.5" checked={notify} onChange={(event) => setNotify(event.target.checked)} />
            <span className="flex flex-col">
              <span className="text-body-sm text-fg">{t.notify}</span>
              <span className="text-caption text-fg-3">{t.notifyHint}</span>
            </span>
          </label>
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          <p className="text-body-sm text-fg-2">{t.instructions}</p>
          <div className="flex flex-wrap gap-2">
            {COLUMNS.map((column) => (
              <Tag key={column}>{column}</Tag>
            ))}
          </div>
        </div>
      )}
    </Dialog>
  );
}
