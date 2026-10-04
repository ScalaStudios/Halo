"use client";

import { useState } from "react";
import { CodeInput } from "@/components/ui/code-input";
import { cn } from "@/lib/cn";
import { describeFailure } from "./passkey";

export function VerifyCode({ verify, hint, label }: { verify: (code: string) => Promise<unknown>; hint: string; label?: string }) {
  const [code, setCode] = useState("");
  const [status, setStatus] = useState<"idle" | "error" | "success">("idle");
  const [problem, setProblem] = useState<string | null>(null);

  function change(next: string) {
    setCode(next);
    if (next) setStatus((current) => (current === "error" ? "idle" : current));
  }

  async function complete(value: string) {
    try {
      await verify(value);
      setProblem(null);
      setStatus("success");
    } catch (error) {
      setProblem(describeFailure(error));
      setStatus("error");
    }
  }

  return (
    <div className="flex flex-col items-center gap-3 text-center">
      <CodeInput value={code} onChange={change} onComplete={complete} status={status} label={label} autoFocus />
      <p aria-live="polite" className={cn("text-body-sm", problem ? "text-danger" : status === "success" ? "text-success" : "text-fg-3")}>
        {problem ?? (status === "success" ? "Code accepted." : hint)}
      </p>
    </div>
  );
}
