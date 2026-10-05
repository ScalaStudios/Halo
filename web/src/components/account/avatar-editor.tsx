"use client";

import { ImageUp, Trash2 } from "lucide-react";
import { useRef, type ReactNode } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { Avatar } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { toApiError } from "@/lib/api/error";

const TYPES = ["image/png", "image/jpeg", "image/webp"];
const SIDE = 256;

async function square(file: File): Promise<Blob> {
  const bitmap = await createImageBitmap(file);
  const side = Math.min(bitmap.width, bitmap.height);
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
  children,
}: {
  name: string;
  avatarUrl: string | null;
  endpoint: string;
  size?: "lg" | "xl";
  children: ReactNode;
}) {
  const { pending, run, toast } = useMutation();
  const input = useRef<HTMLInputElement>(null);

  function upload(file: File) {
    if (!TYPES.includes(file.type)) {
      toast({ title: "That file can't be used", description: "Choose a PNG, JPEG or WebP image.", tone: "danger" });
      return;
    }
    void run(
      "upload",
      async () => {
        const res = await fetch(endpoint, { method: "PUT", body: await square(file), credentials: "same-origin" });
        if (!res.ok) throw await toApiError(res, "Halo could not store the picture. Try again.");
      },
      () => toast({ title: "Profile picture updated", description: "Applications that read the picture claim get it the next time they sign you in." }),
    );
  }

  function remove() {
    void run(
      "remove",
      async () => {
        const res = await fetch(endpoint, { method: "DELETE", credentials: "same-origin" });
        if (!res.ok) throw await toApiError(res, "Halo could not remove the picture. Try again.");
      },
      () => toast({ title: "Profile picture removed" }),
    );
  }

  return (
    <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
      <div className="flex min-w-0 flex-1 items-center gap-4">
        <Avatar name={name} src={avatarUrl} size={size} />
        {children}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        <input
          ref={input}
          type="file"
          accept={TYPES.join(",")}
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
          {avatarUrl ? "Change picture" : "Upload picture"}
        </Button>
        {avatarUrl ? (
          <Button variant="quiet" size="sm" loading={pending === "remove"} onClick={remove}>
            <Trash2 aria-hidden="true" size={16} strokeWidth={1.75} />
            Remove
          </Button>
        ) : null}
      </div>
    </div>
  );
}
