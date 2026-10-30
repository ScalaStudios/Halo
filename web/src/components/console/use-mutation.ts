"use client";

import { useRouter } from "next/navigation";
import { startTransition, useState } from "react";
import { useToast } from "@/components/ui/toast";
import { ApiError } from "@/lib/api/client";

export function useMutation() {
  const router = useRouter();
  const toast = useToast();
  const [pending, setPending] = useState<string | null>(null);

  async function run<T>(key: string, request: () => Promise<T>, onSuccess?: (result: T) => void, onError?: (error: unknown) => boolean): Promise<boolean> {
    setPending(key);
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
      if (onError?.(error)) return false;
      toast({ title: "That didn't work", description: error instanceof ApiError ? error.message : "Halo could not reach the server. Try again.", tone: "danger" });
      return false;
    }
  }

  return { pending, run, toast, router };
}
