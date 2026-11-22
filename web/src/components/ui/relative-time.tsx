"use client";

import { formatDateTime, formatRelative } from "@/lib/format";
import { useLocale } from "@/lib/i18n/client";
import type { Locale } from "@/lib/i18n/locale";

export function RelativeTime({ iso, lowercase, locale: chosen }: { iso: string; lowercase?: boolean; locale?: Locale }) {
  const current = useLocale();
  const locale = chosen ?? current;
  const text = formatRelative(iso, locale);
  return (
    <time dateTime={iso} title={formatDateTime(iso, locale)} suppressHydrationWarning>
      {lowercase ? text.toLowerCase() : text}
    </time>
  );
}
