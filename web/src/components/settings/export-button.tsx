"use client";

import { Download } from "lucide-react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Button, type ButtonSize, type ButtonVariant } from "@/components/ui/button";
import { send } from "@/lib/api/client";
import { useMessages } from "@/lib/i18n/client";
import { settingsMessages } from "@/lib/i18n/console/settings";

async function fetchExport(path: string, failed: string) {
  const res = await send(path, {}, failed);
  const name = /filename="([^"]+)"/.exec(res.headers.get("content-disposition") ?? "")?.[1] ?? "halo-export.json";
  return { name, blob: await res.blob() };
}

export function ExportButton({
  disabled,
  path = "/export",
  label,
  variant = "primary",
  size,
  recorded = true,
}: {
  disabled?: boolean;
  path?: string;
  label?: string;
  variant?: ButtonVariant;
  size?: ButtonSize;
  recorded?: boolean;
}) {
  const { pending, run, errorFor, toast } = useMutation();
  const t = useMessages(settingsMessages).exportButton;

  function download() {
    void run("export", () => fetchExport(path, t.buildFailed), ({ name, blob }) => {
      const url = URL.createObjectURL(blob);
      Object.assign(document.createElement("a"), { href: url, download: name }).click();
      setTimeout(() => URL.revokeObjectURL(url), 60_000);
      toast({ title: t.downloaded, description: t.savedAs(name, recorded) });
    }, "inline");
  }

  return (
    <>
      <Button variant={variant} size={size} loading={pending === "export"} disabled={disabled} onClick={download}>
        <Download aria-hidden="true" size={16} strokeWidth={1.75} />
        {label ?? t.download}
      </Button>
      <ActionError message={errorFor("export")} className="mt-3" />
    </>
  );
}
