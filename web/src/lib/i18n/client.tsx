"use client";

import { createContext, useContext, type ReactNode } from "react";
import type { Locale } from "./locale";

const LocaleContext = createContext<Locale>("en");

export function LocaleProvider({ locale, children }: { locale: Locale; children: ReactNode }) {
  return <LocaleContext.Provider value={locale}>{children}</LocaleContext.Provider>;
}

export function useLocale() {
  return useContext(LocaleContext);
}

export function useMessages<T>(messages: Record<Locale, T>) {
  return messages[useLocale()];
}
