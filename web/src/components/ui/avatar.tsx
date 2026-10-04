import { cn } from "@/lib/cn";

const sizes = {
  sm: "size-6 text-caption",
  md: "size-8 text-caption",
  lg: "size-12 text-body",
  xl: "size-16 text-h4",
} as const;

export function initials(name: string): string {
  const parts = name.trim().split(/\s+/);
  const first = parts[0]?.[0] ?? "";
  const last = parts.length > 1 ? (parts[parts.length - 1]?.[0] ?? "") : "";
  return (first + last).toUpperCase();
}

export function Avatar({
  name,
  size = "md",
  className,
}: {
  name: string;
  size?: keyof typeof sizes;
  className?: string;
}) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "inline-grid shrink-0 place-items-center rounded-full border border-border-strong bg-n800 font-semibold text-fg-2 select-none",
        sizes[size],
        className,
      )}
    >
      {initials(name)}
    </span>
  );
}
