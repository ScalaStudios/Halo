"use client";

import { CircleAlert, Clock, type LucideIcon } from "lucide-react";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { buttonClasses } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { api, ApiError } from "@/lib/api/client";
import { describeFailure } from "./passkey";

type Stop = { icon: LucideIcon; title: string; body: string };

function stopFor(error: unknown): Stop {
  if (error instanceof ApiError && error.code === "ERR_LINK_EXPIRED") {
    return { icon: Clock, title: "This sign-in link has expired", body: "It may have been used already. Request a new one from the sign-in page." };
  }
  if (error instanceof ApiError && error.code === "ERR_AUTH_REQUEST_EXPIRED") return { icon: Clock, title: "This sign-in request expired", body: error.message };
  return { icon: CircleAlert, title: "Halo couldn't sign you in", body: describeFailure(error) };
}

export function MagicLinkRedeem({ token }: { token: string }) {
  const started = useRef(false);
  const [stop, setStop] = useState<Stop | null>(
    token ? null : { icon: CircleAlert, title: "This sign-in link is incomplete", body: "Open the link from your email again, or request a new one from the sign-in page." },
  );

  useEffect(() => {
    if (!token || started.current) return;
    started.current = true;
    api<{ redirect: string }>("/auth/magic-link/redeem", { body: { token } })
      .then(({ redirect }) => window.location.replace(redirect))
      .catch((error) => setStop(stopFor(error)));
  }, [token]);

  if (!stop) {
    return (
      <div role="status" className="flex animate-enter items-center justify-center gap-3 text-body-sm text-fg-3">
        <Spinner size={20} />
        Signing you in
      </div>
    );
  }
  const { icon: Icon, title, body } = stop;
  return (
    <div className="flex animate-enter flex-col items-center gap-6 text-center">
      <div className="flex flex-col items-center gap-3">
        <Icon aria-hidden="true" size={24} strokeWidth={1.75} className="text-fg-3" />
        <h1 className="text-h3 text-fg">{title}</h1>
        <p className="text-body text-fg-3">{body}</p>
      </div>
      <Link href="/sign-in" className={buttonClasses("secondary", "lg", "w-full")}>
        Back to sign in
      </Link>
    </div>
  );
}
