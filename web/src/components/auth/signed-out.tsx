"use client";

import { CircleAlert, CircleCheck } from "lucide-react";
import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { Button, buttonClasses } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { api } from "@/lib/api/client";
import { describeFailure } from "./passkey";

export function SignedOut({ next }: { next?: string }) {
  const started = useRef(false);
  const [done, setDone] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);

  const signOut = useCallback(() => {
    setProblem(null);
    api("/auth/sign-out", { method: "POST" })
      .then(() => (next ? window.location.replace(`/oauth2/logout/return?next=${encodeURIComponent(next)}`) : setDone(true)))
      .catch((error) => setProblem(describeFailure(error)));
  }, [next]);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    signOut();
  }, [signOut]);

  if (problem) {
    return (
      <div className="flex animate-enter flex-col items-center gap-6 text-center">
        <div role="alert" className="flex flex-col items-center gap-3">
          <CircleAlert aria-hidden="true" size={24} strokeWidth={1.75} className="text-danger" />
          <h1 className="text-h3 text-fg">Halo couldn&apos;t sign you out</h1>
          <p className="text-body text-fg-3">{problem}</p>
        </div>
        <Button variant="primary" size="lg" className="w-full" onClick={signOut}>
          Try again
        </Button>
      </div>
    );
  }
  if (!done) {
    return (
      <div role="status" className="flex animate-enter items-center justify-center gap-3 text-body-sm text-fg-3">
        <Spinner size={20} />
        Signing you out of Halo
      </div>
    );
  }
  return (
    <div className="flex animate-enter flex-col items-center gap-6 text-center">
      <div role="status" className="flex flex-col items-center gap-3">
        <CircleCheck aria-hidden="true" size={24} strokeWidth={1.75} className="text-success" />
        <h1 className="text-h3 text-fg">You&apos;re signed out of Halo</h1>
        <p className="text-body text-fg-3">Applications you opened through Halo keep their own sessions until you sign out of them too.</p>
      </div>
      <Link href="/sign-in" className={buttonClasses("secondary", "lg", "w-full")}>
        Sign in again
      </Link>
    </div>
  );
}
