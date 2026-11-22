import { formatDateTime, formatRelative } from "@/lib/format";
import type { Locale } from "@/lib/i18n/locale";

export function RelativeTime({ iso, lowercase, locale = "en" }: { iso: string; lowercase?: boolean; locale?: Locale }) {
  const text = formatRelative(iso, locale);
  return (
    <time dateTime={iso} title={formatDateTime(iso, locale)} suppressHydrationWarning>
      {lowercase ? text.toLowerCase() : text}
    </time>
  );
}
