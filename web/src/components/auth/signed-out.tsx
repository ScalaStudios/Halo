"use client";

import { CircleAlert, CircleCheck } from "lucide-react";
import Link from "next/link";
import { useCallback, useEffect, useRef, useState } from "react";
import { Button, buttonClasses } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { api } from "@/lib/api/client";
import { authMessages } from "@/lib/i18n/auth";
import { useLocale } from "@/lib/i18n/client";
import { describeFailure } from "./passkey";

export function SignedOut({ next, confirm = false }: { next?: string; confirm?: boolean }) {
  const messages = authMessages(useLocale());
  const t = messages.signedOut;
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
          <h1 className="text-h3 text-fg">{t.confirm}</h1>
          <p className="text-body text-fg-3">{t.confirmHint}</p>
        </div>
        <div className="flex w-full flex-col gap-3">
          <Button variant="primary" size="lg" className="w-full" onClick={() => setConfirmed(true)}>
            {t.signOut}
          </Button>
          <Link href="/" className={buttonClasses("secondary", "lg", "w-full")}>
            {t.stay}
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
          <h1 className="text-h3 text-fg">{t.failed}</h1>
          <p className="text-body text-fg-3">{problem}</p>
        </div>
        <Button variant="primary" size="lg" className="w-full" onClick={signOut}>
          {messages.tryAgain}
        </Button>
      </div>
    );
  }
  if (!done) {
    return (
      <div role="status" className="flex animate-enter items-center justify-center gap-3 text-body-sm text-fg-3">
        <Spinner size={20} />
        {t.signingOut}
      </div>
    );
  }
  return (
    <div className="flex animate-enter flex-col items-center gap-6 text-center">
      <div role="status" className="flex flex-col items-center gap-3">
        <CircleCheck aria-hidden="true" size={24} strokeWidth={1.75} className="text-success" />
        <h1 className="text-h3 text-fg">{t.done}</h1>
        <p className="text-body text-fg-3">{t.doneHint}</p>
      </div>
      <Link href="/sign-in" className={buttonClasses("secondary", "lg", "w-full")}>
        {t.signInAgain}
      </Link>
    </div>
  );
}
