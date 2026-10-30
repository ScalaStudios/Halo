"use client";

import { useLinkStatus } from "next/link";
import { createPortal } from "react-dom";

export function NavigationProgress() {
  return (
    <div aria-hidden="true" className="pointer-events-none fixed inset-x-0 top-0 z-[60] h-0.5 overflow-hidden">
      <div className="h-full w-2/5 animate-progress bg-[linear-gradient(90deg,transparent,var(--color-ember),transparent)] motion-reduce:w-full motion-reduce:animate-none motion-reduce:bg-ember motion-reduce:bg-none" />
    </div>
  );
}

export function LinkProgress() {
  const { pending } = useLinkStatus();
  return pending ? createPortal(<NavigationProgress />, document.body) : null;
}
