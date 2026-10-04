import type { ReactNode } from "react";
import { cn } from "@/lib/cn";

export type Tone = "success" | "warning" | "danger" | "info" | "neutral" | "ember";

const tones: Record<Tone, string> = {
  success: "bg-success-container text-success border-success/20",
  warning: "bg-warning-container text-warning border-warning/20",
  danger: "bg-danger-container text-danger border-danger/20",
  info: "bg-info-container text-info border-info/20",
  neutral: "bg-n800 text-fg-2 border-border-strong",
  ember: "bg-ghost text-ember-tint border-ember/20",
};

const dots: Record<Tone, string> = {
  success: "bg-success",
  warning: "bg-warning",
  danger: "bg-danger",
  info: "bg-info",
  neutral: "bg-n400",
  ember: "bg-ember",
};

export function Badge({
  tone = "neutral",
  dot = false,
  children,
  className,
}: {
  tone?: Tone;
  dot?: boolean;
  children: ReactNode;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex h-6 shrink-0 items-center gap-2 rounded-full border px-2 text-label whitespace-nowrap",
        tones[tone],
        className,
      )}
    >
      {dot ? <span aria-hidden="true" className={cn("size-2 rounded-full", dots[tone])} /> : null}
      {children}
    </span>
  );
}

export function StatusDot({ tone, className }: { tone: Tone; className?: string }) {
  return <span aria-hidden="true" className={cn("inline-block size-2 shrink-0 rounded-full", dots[tone], className)} />;
}

export function Tag({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <span
      className={cn(
        "inline-flex h-6 items-center rounded-xs border border-border-strong bg-sunken px-2 font-mono text-code-sm text-fg-2 whitespace-nowrap",
        className,
      )}
    >
      {children}
    </span>
  );
}
