"use client";

import { useRouter } from "next/navigation";
import { startTransition, useState } from "react";
import { useToast } from "@/components/ui/toast";
import { ApiError } from "@/lib/api/client";

export function useMutation() {
  const router = useRouter();
  const toast = useToast();
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
      router.refresh();
      const message = error instanceof ApiError ? error.message : "Halo could not reach the server. Try again.";
      if (onError === "inline") setFailure({ key, message });
      else if (!onError?.(error)) toast({ title: "That didn't work", description: message, tone: "danger" });
      return false;
    }
  }

  function errorFor(key: string) {
    return failure?.key === key ? failure.message : undefined;
  }

  return { pending, run, errorFor, clearError: () => setFailure(null), toast, router };
}
