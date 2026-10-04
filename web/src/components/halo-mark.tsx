import { useId } from "react";
import { cn } from "@/lib/cn";

const PATH = "M18 29.351A13.5 13.5 0 1 0 14 29.351L14 23.228A7.5 7.5 0 1 1 18 23.228Z";

export function HaloMark({ tone = "color", className }: { tone?: "color" | "mono"; className?: string }) {
  const id = useId();
  return (
    <svg viewBox="0 0 32 32" aria-hidden="true" className={cn("shrink-0", className)}>
      {tone === "color" ? (
        <defs>
          <linearGradient id={id} x1="0" y1="0" x2="1" y2="1">
            <stop offset="0" stopColor="#F7AD31" />
            <stop offset="0.48" stopColor="#FA7E26" />
            <stop offset="1" stopColor="#FC4F1B" />
          </linearGradient>
        </defs>
      ) : null}
      <path d={PATH} fill={tone === "color" ? `url(#${id})` : "currentColor"} />
    </svg>
  );
}

export function HaloLogo({ className, size = "md" }: { className?: string; size?: "md" | "lg" }) {
  return (
    <span className={cn("inline-flex items-center", size === "lg" ? "gap-3" : "gap-2", className)}>
      <HaloMark className={size === "lg" ? "size-8" : "size-6"} />
      <span className={cn("font-display font-bold tracking-[-0.01em] text-fg", size === "lg" ? "text-h3" : "text-h4")}>Halo</span>
    </span>
  );
}
