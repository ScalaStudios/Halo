"use client";

import { Info } from "lucide-react";
import type { ReactNode } from "react";
import { useMutation } from "@/components/console/use-mutation";
import { api } from "@/lib/api/client";
import type { Settings, SettingsResponse } from "@/lib/settings-types";

export function useSaveSettings() {
  const { pending, run, toast } = useMutation();
  function save(body: Partial<Settings>, title: string, description: string) {
    return run("save", () => api<SettingsResponse>("/settings", { method: "PATCH", body }), () => toast({ title, description }));
  }
  return { saving: pending === "save", save };
}

export function ReadOnlyNote({ children }: { children: ReactNode }) {
  return (
    <p role="note" className="flex items-start gap-2 text-body-sm text-fg-3">
      <Info aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0" />
      {children}
    </p>
  );
}
