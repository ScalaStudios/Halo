"use client";

import { Eye, EyeOff } from "lucide-react";
import { useState, type ReactNode } from "react";
import { cn } from "@/lib/cn";
import { CopyButton } from "./copy-button";

export function CopyField({
  label,
  value,
  secret = false,
  hint,
  className,
}: {
  label: string;
  value: string;
  secret?: boolean;
  hint?: ReactNode;
  className?: string;
}) {
  const [revealed, setRevealed] = useState(false);
  const hidden = secret && !revealed;

  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <span className="text-label text-fg-3">{label}</span>
      <div className="flex min-h-10 items-center gap-1 rounded-md border border-border bg-sunken py-1 pr-1 pl-4">
        <code className="min-w-0 flex-1 truncate font-mono text-code-sm text-fg" title={hidden ? undefined : value}>
          {hidden ? "•".repeat(32) : value}
        </code>
        {secret ? (
          <button
            type="button"
            onClick={() => setRevealed((current) => !current)}
            aria-label={revealed ? `Hide ${label.toLowerCase()}` : `Reveal ${label.toLowerCase()}`}
            className="grid size-8 shrink-0 place-items-center rounded-md text-fg-3 transition-colors duration-fast hover:bg-hover hover:text-fg"
          >
            {revealed ? <EyeOff aria-hidden="true" size={16} strokeWidth={1.75} /> : <Eye aria-hidden="true" size={16} strokeWidth={1.75} />}
          </button>
        ) : null}
        <CopyButton value={value} label={`Copy ${label.toLowerCase()}`} className="[&>span]:sr-only" />
      </div>
      {hint ? <p className="text-caption text-fg-3">{hint}</p> : null}
    </div>
  );
}
