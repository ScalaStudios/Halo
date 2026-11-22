import { Badge, StatusDot } from "@/components/ui/badge";
import { RelativeTime } from "@/components/ui/relative-time";
import { monitoringMessages } from "@/lib/i18n/console/monitoring";
import { requestLocale } from "@/lib/i18n/server";
import { RESULT, signInApp } from "@/lib/labels";
import type { SignInEvent } from "@/lib/types";

export async function SignInRows({ events, appNames = {} }: { events: SignInEvent[]; appNames?: Record<string, string> }) {
  const t = monitoringMessages[await requestLocale()];
  return (
    <div className="relative overflow-x-auto">
      <table className="w-full text-left text-body-sm">
        <caption className="sr-only">{t.signIns}</caption>
        <thead>
          <tr className="border-y border-border text-label text-fg-3">
            <th scope="col" className="h-10 px-3 pl-6 font-semibold">{t.application}</th>
            <th scope="col" className="h-10 px-3 font-semibold">{t.result}</th>
            <th scope="col" className="h-10 px-3 font-semibold">{t.method}</th>
            <th scope="col" className="hidden h-10 px-3 font-semibold md:table-cell">{t.location}</th>
            <th scope="col" className="hidden h-10 px-3 font-semibold lg:table-cell">{t.ipAddress}</th>
            <th scope="col" className="h-10 px-3 pr-6 text-right font-semibold">{t.time}</th>
          </tr>
        </thead>
        <tbody>
          {events.map((event) => (
            <tr key={event.id} className="h-12 border-b border-border last:border-b-0">
              <td className="px-3 pl-6 text-fg">{signInApp(event, appNames)}</td>
              <td className="px-3">
                <span className="inline-flex items-center gap-2 text-fg-2" title={event.reason}>
                  <StatusDot tone={RESULT[event.result].tone} />
                  {t.results[event.result]}
                </span>
              </td>
              <td className="px-3 text-fg-2">
                <span className="inline-flex items-center gap-2 whitespace-nowrap" title={event.impersonatorId ? event.reason : undefined}>
                  {t.methods[event.method]}
                  {event.impersonatorId ? <Badge tone="warning">{t.impersonation}</Badge> : null}
                </span>
              </td>
              <td className="hidden px-3 text-fg-3 md:table-cell">{event.location}</td>
              <td className="hidden px-3 font-mono text-code-sm text-fg-3 lg:table-cell">{event.ip}</td>
              <td className="tnum px-3 pr-6 text-right whitespace-nowrap text-fg-3"><RelativeTime iso={event.time} /></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
