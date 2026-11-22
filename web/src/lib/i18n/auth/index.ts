import type { Locale } from "../locale";
import { en } from "./en";
import { es } from "./es";

const messages: Record<Locale, typeof en> = { en, es };

export type SignInMessages = typeof en;

export function authMessages(locale: Locale): SignInMessages {
  return messages[locale];
}
