import Link from "next/link";
import { ArrowLeft } from "lucide-react";
import type { ReactNode } from "react";
import { cn } from "@/lib/cn";
import { NavigationProgress } from "./navigation-progress";
import { PageHeader } from "./page-header";

const shimmer =
  "animate-shimmer bg-repeat-x bg-[linear-gradient(90deg,var(--color-n800)_25%,var(--color-n700)_50%,var(--color-n800)_75%)] motion-reduce:animate-none motion-reduce:bg-[linear-gradient(var(--color-n800),var(--color-n800))]";

const range = (count: number) => Array.from({ length: count }, (_, i) => i);

export function Skeleton({ className }: { className?: string }) {
  return <span aria-hidden="true" className={cn("block bg-size-[200%_100%]", shimmer, className)} />;
}

export function SkeletonText({ chars, className }: { chars: number; className?: string }) {
  return (
    <span aria-hidden="true" className={cn("rounded-full bg-size-[200%_60%] bg-center text-transparent select-none box-decoration-clone", shimmer, className)}>
      {"00000 ".repeat(Math.max(1, Math.round(chars / 6))).trim()}
    </span>
  );
}

export function PageSkeleton({
  title,
  description,
  back,
  actions,
  heading,
  large = false,
  className,
  children,
}: {
  title?: string;
  description?: string | number;
  back?: { href: string; label: string };
  actions?: ReactNode;
  heading?: ReactNode;
  large?: boolean;
  className?: string;
  children?: ReactNode;
}) {
  const header = heading ?? (
    <PageHeader
      size={large ? "large" : "default"}
      title={
        title ?? (
          <>
            <SkeletonText chars={22} />
            <span className="sr-only">Loading</span>
          </>
        )
      }
      description={typeof description === "number" ? <SkeletonText chars={description} /> : description}
      actions={actions}
    />
  );
  return (
    <div aria-busy="true" className={cn("flex flex-col", large ? "gap-12" : "gap-8", className)}>
      <NavigationProgress />
      {back ? (
        <div className="flex flex-col gap-6">
          <Link href={back.href} className="inline-flex w-fit items-center gap-2 rounded-sm text-body-sm text-fg-3 transition-colors duration-fast hover:text-fg">
            <ArrowLeft aria-hidden="true" size={16} strokeWidth={1.75} />
            {back.label}
          </Link>
          {header}
        </div>
      ) : (
        header
      )}
      {children}
    </div>
  );
}

export function DetailHeaderSkeleton({
  media,
  actions = 1,
  className = "lg:flex-row lg:items-start lg:justify-between",
}: {
  media?: "avatar" | "icon";
  actions?: number;
  className?: string;
}) {
  return (
    <div className={cn("flex flex-col gap-6", className)}>
      <div className="flex min-w-0 items-center gap-4">
        {media ? <Skeleton className={cn("size-16 shrink-0", media === "avatar" ? "rounded-full" : "rounded-lg")} /> : null}
        <div className="flex min-w-0 flex-col gap-1">
          <p className="text-h2">
            <SkeletonText chars={18} />
            <span className="sr-only">Loading</span>
          </p>
          <p className="text-body">
            <SkeletonText chars={30} />
          </p>
        </div>
      </div>
      <div className="flex gap-2">
        {range(actions).map((i) => (
          <Skeleton key={i} className="h-8 w-28 rounded-md" />
        ))}
      </div>
    </div>
  );
}

export function TabsSkeleton({ labels }: { labels: string[] }) {
  return (
    <div aria-hidden="true" className="overflow-hidden border-b border-border">
      <div className="flex min-w-max gap-1">
        {labels.map((label) => (
          <span key={label} className="flex h-12 items-center px-3 text-body-sm font-medium text-fg-3">
            {label}
          </span>
        ))}
      </div>
    </div>
  );
}

const cellWidths = ["w-36", "w-20", "w-24", "w-16", "w-28", "w-20", "w-24"];

export function TableSkeleton({ columns = 5, rows = 8, views = 0, facets = 0, simple = false }: { columns?: number; rows?: number; views?: number; facets?: number; simple?: boolean }) {
  const cell = (i: number) => cn("px-3", simple ? "first:pl-6 last:pr-6" : "first:pl-4 last:pr-4", i > 2 && "max-md:hidden");
  const table = (
    <table aria-hidden="true" className="w-full border-collapse">
      <thead>
        <tr className="border-b border-border">
          {range(columns).map((i) => (
            <th key={i} className={cn("h-10 text-left text-label", cell(i))}>
              <SkeletonText chars={8} />
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {range(rows).map((row) => (
          <tr key={row} className="h-12 border-b border-border last:border-b-0">
            {range(columns).map((i) => (
              <td key={i} className={cell(i)}>
                <Skeleton className={cn("h-3 max-w-full rounded-sm", cellWidths[i % cellWidths.length])} />
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
  if (simple) return <div className="overflow-hidden rounded-lg border border-border bg-surface">{table}</div>;
  return (
    <div className="flex flex-col gap-4">
      {views ? (
        <div aria-hidden="true" className="flex items-center gap-1 overflow-hidden">
          {range(views).map((i) => (
            <span key={i} className="flex h-8 shrink-0 items-center px-3 text-body-sm">
              <SkeletonText chars={[9, 7, 11][i % 3]} />
            </span>
          ))}
        </div>
      ) : null}
      <div aria-hidden="true" className="flex flex-wrap items-center gap-2">
        <span className="h-8 w-full rounded-md border border-border-strong bg-sunken sm:w-72" />
        {range(facets).map((i) => (
          <span key={i} className="h-8 w-24 rounded-md border border-border" />
        ))}
        <span className="ml-auto flex h-8 items-center px-3 text-body-sm">
          <SkeletonText chars={8} />
        </span>
      </div>
      <div className="overflow-hidden rounded-lg border border-border bg-surface">
        {table}
        <div aria-hidden="true" className="flex items-center justify-between gap-4 border-t border-border px-4 py-2 text-body-sm">
          <SkeletonText chars={20} />
          <span className="flex h-8 items-center">
            <SkeletonText chars={11} />
          </span>
        </div>
      </div>
    </div>
  );
}

export function ListSkeleton({ rows = 3, media = false, className }: { rows?: number; media?: boolean; className?: string }) {
  return (
    <ul aria-hidden="true" className={className}>
      {range(rows).map((i) => (
        <li key={i} className="flex items-center gap-4 border-t border-border px-6 py-4 first:border-t-0">
          {media ? <Skeleton className="size-8 shrink-0 rounded-full" /> : null}
          <span className="flex min-w-0 flex-1 flex-col gap-1">
            <span className="text-body-sm">
              <SkeletonText chars={[26, 20, 32][i % 3]} />
            </span>
            <span className="text-body-sm">
              <SkeletonText chars={[44, 36, 40][i % 3]} />
            </span>
          </span>
          <span className="shrink-0 text-caption">
            <SkeletonText chars={10} />
          </span>
        </li>
      ))}
    </ul>
  );
}

export function FieldSkeleton({ label, hint, tall = false }: { label?: string; hint?: string | number; tall?: boolean }) {
  return (
    <div aria-hidden="true" className="flex flex-col gap-2">
      <span className="text-label text-fg-3">{label ?? <SkeletonText chars={16} />}</span>
      <span className={cn("rounded-md border border-border-strong bg-sunken", tall ? "h-28" : "h-10")} />
      {hint ? <span className="text-caption text-fg-3">{typeof hint === "number" ? <SkeletonText chars={hint} /> : hint}</span> : null}
    </div>
  );
}
