import { formatDateTime, formatRelative } from "@/lib/format";

export function RelativeTime({ iso, lowercase }: { iso: string; lowercase?: boolean }) {
  const text = formatRelative(iso);
  return (
    <time dateTime={iso} title={formatDateTime(iso)} suppressHydrationWarning>
      {lowercase ? text.toLowerCase() : text}
    </time>
  );
}
