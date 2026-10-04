"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { useToast } from "@/components/ui/toast";
import { ApiError } from "@/lib/api/client";

export function useMutation() {
  const router = useRouter();
  const toast = useToast();
  const [pending, setPending] = useState<string | null>(null);

  async function run<T>(key: string, request: () => Promise<T>, onSuccess?: (result: T) => void): Promise<boolean> {
    setPending(key);
    try {
      const result = await request();
      onSuccess?.(result);
      return true;
    } catch (error) {
      toast({ title: "That didn't work", description: error instanceof ApiError ? error.message : "Halo could not reach the server. Try again.", tone: "danger" });
      return false;
    } finally {
      setPending(null);
      router.refresh();
    }
  }

  return { pending, run, toast, router };
}
