"use client";

import { useMutation } from "@/components/console/use-mutation";
import { Select } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { api } from "@/lib/api/client";
import { accountMessages } from "@/lib/i18n/account";
import { useLocale } from "@/lib/i18n/client";
import { languageNames, locales, type Locale } from "@/lib/i18n/locale";

export function LanguagePicker({ chosen }: { chosen: Locale | null }) {
  const locale = useLocale();
  const copy = accountMessages(locale);
  const { pending, run, toast } = useMutation();

  function choose(value: string) {
    const next = (value || null) as Locale | null;
    void run("locale", () => api("/me/locale", { method: "PUT", body: { locale: next } }), () => toast({ title: accountMessages(next ?? locale).languageSaved }));
  }

  return (
    <div className="flex items-center gap-3">
      <Select aria-label={copy.language} value={chosen ?? ""} disabled={pending !== null} onChange={(event) => choose(event.target.value)}>
        <option value="">{copy.automatic}</option>
        {locales.map((value) => (
          <option key={value} value={value} lang={value}>
            {languageNames[value]}
          </option>
        ))}
      </Select>
      {pending ? <Spinner className="shrink-0 text-fg-3" /> : null}
    </div>
  );
}
