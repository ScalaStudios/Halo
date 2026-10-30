"use client";

import { useRouter } from "next/navigation";
import { startTransition, useState } from "react";
import { useToast } from "@/components/ui/toast";
import { ApiError } from "@/lib/api/client";

export function useMutation() {
  const router = useRouter();
  const toast = useToast();
  const [pending, setPending] = useState<string | null>(null);
  const [error, setError] = useState<{ key: string; message: string } | null>(null);

  async function run<T>(
    key: string,
    request: () => Promise<T>,
    onSuccess?: (result: T) => void,
    onError?: ((error: unknown) => boolean) | "inline",
  ): Promise<boolean> {
    setPending(key);
    setError(null);
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
      if (onError === "inline") setError({ key, message });
      else if (!onError?.(error)) toast({ title: "That didn't work", description: message, tone: "danger" });
      return false;
    }
  }

  function errorFor(key: string) {
    return error?.key === key ? error.message : undefined;
  }

  return { pending, run, errorFor, clearError: () => setError(null), toast, router };
}
