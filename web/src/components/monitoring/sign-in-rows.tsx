import { StatusDot } from "@/components/ui/badge";
import { RelativeTime } from "@/components/ui/relative-time";
import { METHOD, RESULT, signInApp } from "@/lib/labels";
import type { SignInEvent } from "@/lib/types";

export function SignInRows({ events, appNames = {} }: { events: SignInEvent[]; appNames?: Record<string, string> }) {
  return (
    <div className="relative overflow-x-auto">
      <table className="w-full text-left text-body-sm">
        <caption className="sr-only">Sign-ins</caption>
        <thead>
          <tr className="border-y border-border text-label text-fg-3">
            <th scope="col" className="h-10 px-3 pl-6 font-semibold">Application</th>
            <th scope="col" className="h-10 px-3 font-semibold">Result</th>
            <th scope="col" className="h-10 px-3 font-semibold">Method</th>
            <th scope="col" className="hidden h-10 px-3 font-semibold md:table-cell">Location</th>
            <th scope="col" className="hidden h-10 px-3 font-semibold lg:table-cell">IP address</th>
            <th scope="col" className="h-10 px-3 pr-6 text-right font-semibold">Time</th>
          </tr>
        </thead>
        <tbody>
          {events.map((event) => (
            <tr key={event.id} className="h-12 border-b border-border last:border-b-0">
              <td className="px-3 pl-6 text-fg">{signInApp(event, appNames)}</td>
              <td className="px-3">
                <span className="inline-flex items-center gap-2 text-fg-2" title={event.reason}>
                  <StatusDot tone={RESULT[event.result].tone} />
                  {RESULT[event.result].label}
                </span>
              </td>
              <td className="px-3 text-fg-2">{METHOD[event.method].label}</td>
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
