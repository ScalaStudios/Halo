"use client";

import { useRouter } from "next/navigation";
import { startTransition, useState } from "react";
import { useToast } from "@/components/ui/toast";
import { ApiError, ApprovalPending } from "@/lib/api/client";
import { formatDateTime } from "@/lib/format";
import { useLocale } from "@/lib/i18n/client";
import { frameMessages } from "@/lib/i18n/console/frame";

export function useMutation() {
  const router = useRouter();
  const toast = useToast();
  const locale = useLocale();
  const copy = frameMessages[locale].mutation;
  const [pending, setPending] = useState<string | null>(null);
  const [failure, setFailure] = useState<{ key: string; message: string } | null>(null);

  async function run<T>(
    key: string,
    request: () => Promise<T>,
    onSuccess?: (result: T) => void,
    onError?: ((error: unknown) => boolean) | "inline",
  ): Promise<boolean> {
    setPending(key);
    setFailure(null);
    try {
      const result = await request();
      startTransition(() => {
        onSuccess?.(result);
        setPending(null);
        router.refresh();
      });
      return true;
    } catch (error) {
      setPending(null);
      if (error instanceof ApprovalPending) {
        toast({
          title: copy.waiting,
          description: copy.waitingHint(formatDateTime(error.request.expiresAt, locale)),
        });
        router.push("/admin/approvals");
        router.refresh();
        return false;
      }
      router.refresh();
      const message = error instanceof ApiError ? error.message : copy.unreachable;
      if (onError === "inline") setFailure({ key, message });
      else if (!onError?.(error)) toast({ title: copy.failed, description: message, tone: "danger" });
      return false;
    }
  }

  function errorFor(key: string) {
    return failure?.key === key ? failure.message : undefined;
  }

  return { pending, run, errorFor, clearError: () => setFailure(null), toast, router };
}
