"use client";

import { Download } from "lucide-react";
import { useMutation } from "@/components/console/use-mutation";
import { Button } from "@/components/ui/button";
import { toApiError } from "@/lib/api/error";

async function fetchExport() {
  const res = await fetch("/api/v1/export", { credentials: "same-origin" });
  if (!res.ok) throw await toApiError(res, "Halo could not build the export. Try again.");
  const name = /filename="([^"]+)"/.exec(res.headers.get("content-disposition") ?? "")?.[1] ?? "halo-export.json";
  return { name, blob: await res.blob() };
}

export function ExportButton({ disabled }: { disabled: boolean }) {
  const { pending, run, toast } = useMutation();

  function download() {
    void run("export", fetchExport, ({ name, blob }) => {
      const url = URL.createObjectURL(blob);
      Object.assign(document.createElement("a"), { href: url, download: name }).click();
      setTimeout(() => URL.revokeObjectURL(url), 60_000);
      toast({ title: "Export downloaded", description: `Saved as ${name}. The export is recorded in the audit log.` });
    });
  }

  return (
    <Button variant="primary" loading={pending === "export"} disabled={disabled} onClick={download}>
      <Download aria-hidden="true" size={16} strokeWidth={1.75} />
      Download export
    </Button>
  );
}
