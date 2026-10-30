import { cn } from "@/lib/cn";

const sizes = {
  sm: "size-6 text-caption",
  md: "size-8 text-caption",
  lg: "size-12 text-body",
  xl: "size-16 text-h4",
} as const;

export function initials(name: string): string {
  const parts = name.split(/\s+/).flatMap((word) => word.match(/[\p{L}\p{N}]/u) ?? []);
  const first = parts[0] ?? "";
  const last = parts.length > 1 ? (parts[parts.length - 1] ?? "") : "";
  return (first + last).toUpperCase();
}

export function Avatar({
  name,
  src,
  size = "md",
  square = false,
  className,
}: {
  name: string;
  src?: string | null;
  size?: keyof typeof sizes;
  square?: boolean;
  className?: string;
}) {
  const shape = square ? "rounded-md" : "rounded-full";
  if (src) {
    return <img src={src} alt="" aria-hidden="true" className={cn("shrink-0 border border-border-strong bg-n800", shape, square ? "object-contain" : "object-cover", sizes[size], className)} />;
  }
  return (
    <span
      aria-hidden="true"
      className={cn(
        "inline-grid shrink-0 place-items-center border border-border-strong bg-n800 font-semibold text-fg-2 select-none",
        shape,
        sizes[size],
        className,
      )}
    >
      {initials(name)}
    </span>
  );
}
