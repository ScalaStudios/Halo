"use client";

import { ImageUp, Trash2 } from "lucide-react";
import { useRef, type ReactNode } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { ActionError } from "@/components/ui/action-error";
import { Avatar } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { toApiError } from "@/lib/api/error";
import { cn } from "@/lib/cn";
import { accountMessages } from "@/lib/i18n/account";
import type { Locale } from "@/lib/i18n/locale";

const TYPES = ["image/png", "image/jpeg", "image/webp"];
const SVG = "image/svg+xml";
const SIDE = 256;

async function square(file: File, pad: boolean): Promise<Blob> {
  const bitmap = await createImageBitmap(file);
  const side = (pad ? Math.max : Math.min)(bitmap.width, bitmap.height);
  const canvas = document.createElement("canvas");
  canvas.width = SIDE;
  canvas.height = SIDE;
  canvas.getContext("2d")?.drawImage(bitmap, (bitmap.width - side) / 2, (bitmap.height - side) / 2, side, side, 0, 0, SIDE, SIDE);
  bitmap.close();
  return new Promise((resolve, reject) => canvas.toBlob((blob) => (blob ? resolve(blob) : reject(new Error("encode"))), "image/webp", 0.9));
}

export function AvatarEditor({
  name,
  avatarUrl,
  endpoint,
  size = "lg",
  logo = false,
  locale = "en",
  className,
  children,
}: {
  name: string;
  avatarUrl: string | null;
  endpoint: string;
  size?: "lg" | "xl";
  logo?: boolean;
  locale?: Locale;
  className?: string;
  children: ReactNode;
}) {
  const { pending, run, errorFor, toast } = useMutation();
  const input = useRef<HTMLInputElement>(null);
  const types = logo ? [...TYPES, SVG] : TYPES;
  const copy = accountMessages(locale).picture;

  function upload(file: File) {
    if (!types.includes(file.type)) {
      toast({ title: copy.badFile, description: logo ? "Choose a PNG, JPEG, WebP or SVG image." : copy.chooseFile, tone: "danger" });
      return;
    }
    void run(
      "upload",
      async () => {
        const res = await fetch(endpoint, { method: "PUT", body: file.type === SVG ? file : await square(file, logo), credentials: "same-origin" });
        if (!res.ok) throw await toApiError(res, logo ? "Halo could not store the logo. Try again." : copy.uploadFailed);
      },
      () => toast(logo ? { title: "Logo updated" } : { title: copy.updated, description: copy.updatedHint }),
      "inline",
    );
  }

  function remove() {
    void run(
      "remove",
      async () => {
        const res = await fetch(endpoint, { method: "DELETE", credentials: "same-origin" });
        if (!res.ok) throw await toApiError(res, logo ? "Halo could not remove the logo. Try again." : copy.removeFailed);
      },
      () => toast({ title: logo ? "Logo removed" : copy.removed }),
      "inline",
    );
  }

  return (
    <div className={cn("flex flex-col gap-4 sm:flex-row sm:items-center", className)}>
      <div className="flex min-w-0 flex-1 items-center gap-4">
        <Avatar name={name} src={avatarUrl} size={size} square={logo} />
        {children}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <input
          ref={input}
          type="file"
          accept={types.join(",")}
          className="sr-only"
          tabIndex={-1}
          aria-hidden="true"
          onChange={(event) => {
            const file = event.target.files?.[0];
            event.target.value = "";
            if (file) upload(file);
          }}
        />
        <Button variant="secondary" size="sm" loading={pending === "upload"} onClick={() => input.current?.click()}>
          <ImageUp aria-hidden="true" size={16} strokeWidth={1.75} />
          {avatarUrl ? (logo ? "Change logo" : copy.change) : (logo ? "Upload logo" : copy.upload)}
        </Button>
        {avatarUrl ? (
          <Button variant="quiet" size="sm" loading={pending === "remove"} onClick={remove}>
            <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
            {copy.remove}
          </Button>
        ) : null}
        <ActionError message={errorFor("upload") ?? errorFor("remove")} className="basis-full" />
      </div>
    </div>
  );
}
