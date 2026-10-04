import Link from "next/link";
import { HaloLogo } from "@/components/halo-mark";
import { buttonClasses } from "@/components/ui/button";

export default function NotFound() {
  return (
    <main className="grid min-h-dvh place-items-center px-4 py-16">
      <div className="flex animate-page flex-col items-center gap-6 text-center">
        <HaloLogo size="lg" />
        <div className="flex flex-col gap-2">
          <h1 className="text-h2 text-fg">Page not found</h1>
          <p className="max-w-md text-body text-fg-3">The address may be mistyped, or the page was moved or removed.</p>
        </div>
        <Link href="/admin" className={buttonClasses("primary", "md")}>
          Go to Overview
        </Link>
      </div>
    </main>
  );
}
