"use client";

import { CircleAlert, Clock, type LucideIcon } from "lucide-react";
import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { buttonClasses } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { api, ApiError } from "@/lib/api/client";
import { authMessages, type SignInMessages } from "@/lib/i18n/auth";
import type { Locale } from "@/lib/i18n/locale";
import { describeFailure } from "./passkey";

type Stop = { icon: LucideIcon; title: string; body: string; bodyLocale?: Locale };

function stopFor(error: unknown, messages: SignInMessages): Stop {
  if (error instanceof ApiError && error.code === "ERR_LINK_EXPIRED") {
    return { icon: Clock, title: messages.magicLink.expired, body: messages.magicLink.expiredHint };
  }
  if (error instanceof ApiError && error.code === "ERR_AUTH_REQUEST_EXPIRED") return { icon: Clock, title: messages.expired, body: error.message, bodyLocale: "en" };
  return { icon: CircleAlert, title: messages.magicLink.failed, body: describeFailure(error), bodyLocale: "en" };
}

export function MagicLinkRedeem({ token, locale }: { token: string; locale: Locale }) {
  const copy = authMessages(locale);
  const messages = copy.magicLink;
  const started = useRef(false);
  const [stop, setStop] = useState<Stop | null>(
    token ? null : { icon: CircleAlert, title: messages.incomplete, body: messages.incompleteHint },
  );

  useEffect(() => {
    if (!token || started.current) return;
    started.current = true;
    api<{ redirect: string }>("/auth/magic-link/redeem", { body: { token } })
      .then(({ redirect }) => window.location.replace(redirect))
      .catch((error) => setStop(stopFor(error, copy)));
  }, [token, copy]);

  if (!stop) {
    return (
      <div lang={locale} role="status" className="flex animate-enter items-center justify-center gap-3 text-body-sm text-fg-3">
        <Spinner size={20} />
        {messages.redeeming}
      </div>
    );
  }
  const { icon: Icon, title, body, bodyLocale } = stop;
  return (
    <div lang={locale} className="flex animate-enter flex-col items-center gap-6 text-center">
      <div className="flex flex-col items-center gap-3">
        <Icon aria-hidden="true" size={24} strokeWidth={1.75} className="text-fg-3" />
        <h1 className="text-h3 text-fg">{title}</h1>
        <p lang={bodyLocale} className="text-body text-fg-3">{body}</p>
      </div>
      <Link href="/sign-in" className={buttonClasses("secondary", "lg", "w-full")}>
        {copy.backToSignIn}
      </Link>
    </div>
  );
}
