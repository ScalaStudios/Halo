import Link from "next/link";
import { HaloLogo } from "@/components/halo-mark";
import { buttonClasses } from "@/components/ui/button";
import { authMessages } from "@/lib/i18n/auth";
import { requestLocale } from "@/lib/i18n/server";

export default async function NotFound() {
  const t = authMessages(await requestLocale()).notFound;
  return (
    <main className="grid min-h-dvh place-items-center px-4 py-16">
      <div className="flex animate-page flex-col items-center gap-6 text-center">
        <HaloLogo size="lg" />
        <div className="flex flex-col gap-2">
          <h1 className="text-h2 text-fg">{t.title}</h1>
          <p className="max-w-md text-body text-fg-3">{t.description}</p>
        </div>
        <Link href="/admin" className={buttonClasses("primary", "md")}>
          {t.home}
        </Link>
      </div>
    </main>
  );
}
