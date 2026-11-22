import { headers } from "next/headers";
import { preferredLocale } from "./locale";

export async function requestLocale() {
  return preferredLocale((await headers()).get("accept-language"));
}
