"use client";

import { DropdownMenu as M } from "radix-ui";
import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { cn } from "@/lib/cn";

export const Menu = M.Root;
export const MenuTrigger = M.Trigger;

export function MenuContent({
  children,
  align = "end",
  className,
}: {
  children: ReactNode;
  align?: "start" | "center" | "end";
  className?: string;
}) {
  return (
    <M.Portal>
      <M.Content
        align={align}
        sideOffset={4}
        collisionPadding={16}
        className={cn(
          "z-50 min-w-48 rounded-md border border-border bg-surface p-1 shadow-md outline-none",
          "data-[state=open]:animate-[halo-pop_160ms_var(--ease-brand)]",
          className,
        )}
      >
        {children}
      </M.Content>
    </M.Portal>
  );
}

export function MenuItem({
  icon: Icon,
  children,
  shortcut,
  danger = false,
  onSelect,
  disabled,
}: {
  icon?: LucideIcon;
  children: ReactNode;
  shortcut?: ReactNode;
  danger?: boolean;
  onSelect?: (event: Event) => void;
  disabled?: boolean;
}) {
  return (
    <M.Item
      onSelect={onSelect}
      disabled={disabled}
      className={cn(
        "flex h-8 cursor-pointer items-center gap-2 rounded-sm px-2 text-body-sm outline-none select-none",
        "data-highlighted:bg-press data-disabled:pointer-events-none data-disabled:opacity-45",
        danger ? "text-danger" : "text-fg-2 data-highlighted:text-fg",
      )}
    >
      {Icon ? <Icon aria-hidden="true" size={16} strokeWidth={1.75} className={danger ? "text-danger" : "text-fg-3"} /> : null}
      <span className="flex-1">{children}</span>
      {shortcut ? <span className="text-caption text-fg-3">{shortcut}</span> : null}
    </M.Item>
  );
}

export function MenuCheckboxItem({
  checked,
  onCheckedChange,
  children,
}: {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  children: ReactNode;
}) {
  return (
    <M.CheckboxItem
      checked={checked}
      onCheckedChange={onCheckedChange}
      onSelect={(event) => event.preventDefault()}
      className="flex h-8 cursor-pointer items-center gap-2 rounded-sm px-2 text-body-sm text-fg-2 outline-none select-none data-highlighted:bg-press data-highlighted:text-fg"
    >
      <span
        aria-hidden="true"
        className={cn(
          "grid size-4 place-items-center rounded-sm border",
          checked ? "border-ember bg-ember text-ink" : "border-border-strong bg-sunken",
        )}
      >
        <M.ItemIndicator>
          <svg viewBox="0 0 16 16" className="size-4" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M3.5 8.5l3 3 6-6.5" />
          </svg>
        </M.ItemIndicator>
      </span>
      {children}
    </M.CheckboxItem>
  );
}

export function MenuLabel({ children }: { children: ReactNode }) {
  return <M.Label className="px-2 pt-2 pb-1 text-label text-fg-3">{children}</M.Label>;
}

export function MenuSeparator() {
  return <M.Separator className="my-1 h-px bg-border" />;
}
