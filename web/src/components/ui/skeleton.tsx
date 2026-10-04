import { cn } from "@/lib/cn";

export function Skeleton({ className }: { className?: string }) {
  return (
    <div
      aria-hidden="true"
      className={cn(
        "h-3 animate-shimmer rounded-sm bg-[linear-gradient(90deg,var(--color-n800)_25%,var(--color-n700)_50%,var(--color-n800)_75%)] bg-size-[200%_100%]",
        className,
      )}
    />
  );
}
