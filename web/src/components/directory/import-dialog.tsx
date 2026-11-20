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
import { pluralize } from "@/lib/format";
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
  const details = [row.title, row.department, row.location, row.managerEmail && `Reports to ${row.managerEmail}`, row.groups.join(", ")].filter(Boolean);
  return (
    <span className="flex flex-col">
      <span className="font-medium text-fg">{row.name || "No name"}</span>
      <span className="text-caption break-all text-fg-3">{row.email || "No email"}</span>
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
          "Halo could not read the file. Try again.",
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
      title={done ? "People imported" : preview ? "Check the import" : "Import users"}
      description={
        done
          ? `Created ${pluralize(created.length, "person", "people")} from ${file?.name}. Each one is invited and appears in the directory.`
          : preview
            ? `Nothing is created until you confirm. Rows with a problem are skipped, so fix them in ${file?.name} and choose it again, or import the rest.`
            : "Upload a CSV file to invite many people at once. Halo checks every row and shows you the result before anyone is created."
      }
      footer={
        done ? (
          <Button variant="primary" onClick={close}>
            Done
          </Button>
        ) : (
          <>
            <Button variant="secondary" onClick={close}>
              Cancel
            </Button>
            {preview ? (
              <Button variant="primary" loading={pending === "import"} disabled={preview.ready === 0 || pending === "preview"} onClick={() => file && upload(file, false)}>
                Import {pluralize(preview.ready, "person", "people")}
              </Button>
            ) : (
              <Button variant="primary" loading={pending === "preview"} onClick={() => input.current?.click()}>
                Choose CSV file
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
          {held.length ? (
            <SecretWarning>
              Halo didn&apos;t email {held.length === created.length ? "the" : `${held.length} of the`} setup links, so copy each one and send it yourself. Anyone with a link can
              enroll a passkey on that account.
            </SecretWarning>
          ) : (
            <p className="text-body-sm text-fg-2">Halo emailed each person their setup link.</p>
          )}
          <SimpleTable
            caption="People created by the import"
            head={["Person", "Setup link"]}
            rows={created.map((row) => [
              <Person key="person" row={row} />,
              row.emailed ? <span className="whitespace-nowrap text-fg-3">Emailed</span> : <CopyButton value={row.enrollUrl ?? ""} label="Copy link" className="whitespace-nowrap" />,
            ])}
          />
        </div>
      ) : preview ? (
        <div className="flex animate-enter flex-col gap-6">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <p aria-live="polite" className="text-body-sm text-fg-2">
              {pluralize(preview.ready, "person", "people")} ready to import
              {preview.skipped ? `, ${pluralize(preview.skipped, "row")} with a problem will be skipped` : ""}.
            </p>
            <Button size="sm" variant="secondary" loading={pending === "preview"} disabled={pending === "import"} onClick={() => input.current?.click()}>
              Choose another file
            </Button>
          </div>
          <SimpleTable
            caption={`Rows in ${file?.name ?? "the file"}`}
            head={["Row", "Person"]}
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
                      {row.error.message}
                    </span>
                  ) : (
                    <span className="mt-1 flex items-center gap-2 text-success">
                      <CircleCheck aria-hidden="true" size={16} strokeWidth={1.75} className="shrink-0" />
                      Ready
                    </span>
                  )}
                </Person>,
              ])}
          />
          <label className="flex cursor-pointer items-start gap-3">
            <Checkbox className="mt-0.5" checked={notify} onChange={(event) => setNotify(event.target.checked)} />
            <span className="flex flex-col">
              <span className="text-body-sm text-fg">Email each person their setup link</span>
              <span className="text-caption text-fg-3">Clear this to hold the invitations. Halo then shows the setup links for you to share yourself.</span>
            </span>
          </label>
        </div>
      ) : (
        <div className="flex flex-col gap-3">
          <p className="text-body-sm text-fg-2">
            The first row names the columns. Email and name are required. Separate several groups with semicolons. Up to 500 people and 1 MB per file. Imports never grant
            administrator roles.
          </p>
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
