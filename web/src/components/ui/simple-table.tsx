import type { ReactNode } from "react";

export function SimpleTable({ caption, head, rows }: { caption: string; head: string[]; rows: ReactNode[][] }) {
  return (
    <div className="relative overflow-x-auto rounded-lg border border-border bg-surface">
      <table className="w-full text-left text-body-sm">
        <caption className="sr-only">{caption}</caption>
        <thead>
          <tr className="border-b border-border">
            {head.map((h) => (
              <th key={h} scope="col" className="h-10 px-3 text-label whitespace-nowrap text-fg-3 first:pl-6 last:pr-6">
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((cells, i) => (
            <tr key={i} className="h-12 border-b border-border last:border-b-0">
              {cells.map((cell, j) => (
                <td key={j} className="px-3 py-2 text-fg-2 first:pl-6 last:pr-6">
                  {cell}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
