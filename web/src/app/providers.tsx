"use client";

import type { ReactNode } from "react";
import { ReauthPrompt } from "@/components/auth/reauth-prompt";
import { ToastProvider } from "@/components/ui/toast";
import { TooltipProvider } from "@/components/ui/tooltip";

export function Providers({ children }: { children: ReactNode }) {
  return (
    <TooltipProvider>
      <ToastProvider>
        {children}
        <ReauthPrompt />
      </ToastProvider>
    </TooltipProvider>
  );
}
