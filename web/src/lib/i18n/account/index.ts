import type { Locale } from "../locale";
import { en } from "./en";
import { es } from "./es";

const messages: Record<Locale, typeof en> = { en, es };

export function accountMessages(locale: Locale) {
  return messages[locale];
}
