import type { ReactNode } from "react";
import { HaloLogo, HaloMark } from "@/components/halo-mark";
import { apiGet } from "@/lib/api/server";
import type { OrganizationProfile } from "@/lib/settings-types";

export async function AuthFrame({ children }: { children: ReactNode }) {
  const org = await apiGet<OrganizationProfile>("/organization");
  return (
    <div className="relative isolate flex min-h-dvh flex-col overflow-hidden">
      <div aria-hidden="true" className="pointer-events-none absolute inset-x-0 top-0 -z-10 h-96 bg-hearth" />
      <main className="flex flex-1 flex-col items-center px-5 pt-24 pb-16 sm:justify-center sm:px-8 sm:pt-16">
        <div className="flex w-full max-w-[400px] flex-col gap-12">
          <div className="flex flex-col items-center gap-4 text-center">
            {org.logoUrl ? (
              <>
                <img src={org.logoUrl} alt="" className="h-12 w-auto max-w-[240px] object-contain" />
                <span className="text-h4 text-fg">{org.name}</span>
              </>
            ) : (
              <HaloLogo size="lg" />
            )}
            {org.signInMessage ? <p className="text-body-sm whitespace-pre-line text-fg-2">{org.signInMessage}</p> : null}
          </div>
          {children}
        </div>
      </main>
      <footer className="flex flex-wrap items-center justify-center gap-x-3 px-5 pb-6 text-caption text-fg-3">
        <span className="inline-flex items-center gap-2">
          {org.logoUrl ? <HaloMark tone="mono" className="size-4" /> : null}
          {org.name} · Halo
        </span>
        {org.contactEmail ? (
          <>
            <span aria-hidden="true">·</span>
            <a href={`mailto:${org.contactEmail}`} className="inline-flex min-h-12 items-center underline-offset-3 transition-colors duration-fast ease-brand hover:text-fg hover:underline">
              Get help
            </a>
          </>
        ) : null}
      </footer>
    </div>
  );
}
