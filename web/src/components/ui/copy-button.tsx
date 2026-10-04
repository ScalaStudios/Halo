"use client";

import { Check, Copy } from "lucide-react";
import { useEffect, useState } from "react";
import { cn } from "@/lib/cn";

export function CopyButton({ value, label = "Copy", className }: { value: string; label?: string; className?: string }) {
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!copied) return;
    const timer = setTimeout(() => setCopied(false), 1600);
    return () => clearTimeout(timer);
  }, [copied]);

  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  }

  return (
    <button
      type="button"
      onClick={copy}
      aria-label={copied ? "Copied" : label}
      className={cn(
        "inline-flex h-8 shrink-0 items-center gap-2 rounded-md px-2 text-body-sm font-medium text-fg-3",
        "transition-colors duration-fast ease-brand hover:bg-hover hover:text-fg",
        copied && "text-success hover:text-success",
        className,
      )}
    >
      {copied ? <Check aria-hidden="true" size={16} strokeWidth={1.75} /> : <Copy aria-hidden="true" size={16} strokeWidth={1.75} />}
      <span aria-live="polite">{copied ? "Copied" : label}</span>
    </button>
  );
}
