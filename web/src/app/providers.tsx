"use client";

import type { ReactNode } from "react";
import { ReauthPrompt } from "@/components/auth/reauth-prompt";
import { ToastProvider } from "@/components/ui/toast";
import { TooltipProvider } from "@/components/ui/tooltip";
import { LocaleProvider } from "@/lib/i18n/client";
import type { Locale } from "@/lib/i18n/locale";

export function Providers({ locale, children }: { locale: Locale; children: ReactNode }) {
  return (
    <LocaleProvider locale={locale}>
      <TooltipProvider>
        <ToastProvider>
          {children}
          <ReauthPrompt />
        </ToastProvider>
      </TooltipProvider>
    </LocaleProvider>
  );
}
