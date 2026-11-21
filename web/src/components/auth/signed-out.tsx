"use client";

import { CircleAlert, CircleCheck } from "lucide-react";
import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { Button, buttonClasses } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { api } from "@/lib/api/client";
import { describeFailure } from "./passkey";

export function SignedOut({ next, confirm = false }: { next?: string; confirm?: boolean }) {
  const started = useRef(false);
  const [confirmed, setConfirmed] = useState(!confirm);
  const [done, setDone] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);

  const signOut = useCallback(() => {
    setProblem(null);
    api("/auth/sign-out", { method: "POST" })
      .then(() => (next ? window.location.replace(`/oauth2/logout/return?next=${encodeURIComponent(next)}`) : setDone(true)))
      .catch((error) => setProblem(describeFailure(error)));
  }, [next]);

  useEffect(() => {
    if (!confirmed || started.current) return;
    started.current = true;
    signOut();
  }, [confirmed, signOut]);

  if (!confirmed) {
    return (
      <div className="flex animate-enter flex-col items-center gap-6 text-center">
        <div className="flex flex-col items-center gap-3">
          <h1 className="text-h3 text-fg">Sign out of Halo?</h1>
          <p className="text-body text-fg-3">An application asked Halo to sign you out. Sign out to end your Halo session on this browser, or stay signed in.</p>
        </div>
        <div className="flex w-full flex-col gap-3">
          <Button variant="primary" size="lg" className="w-full" onClick={() => setConfirmed(true)}>
            Sign out
          </Button>
          <Link href="/" className={buttonClasses("secondary", "lg", "w-full")}>
            Stay signed in
          </Link>
        </div>
      </div>
    );
  }

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
