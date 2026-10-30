"use client";

import { useState, type ReactNode } from "react";
import HoldButton from "@/components/reactbits/HoldButton";
import { Spinner } from "./spinner";

export function HoldToConfirm({
  children,
  doneLabel,
  icon,
  onConfirm,
  size = "md",
  disabled,
  className,
}: {
  children: ReactNode;
  doneLabel: ReactNode;
  icon?: ReactNode;
  onConfirm: () => Promise<boolean>;
  size?: "sm" | "md";
  disabled?: boolean;
  className?: string;
}) {
  const [busy, setBusy] = useState(false);
  const [attempt, setAttempt] = useState(0);

  async function confirm() {
    setBusy(true);
    const done = await onConfirm();
    setBusy(false);
    if (!done) setAttempt((current) => current + 1);
  }

  return (
    <HoldButton
      key={attempt}
      size={size}
      icon={icon}
      doneLabel={busy ? <Spinner className="text-white" /> : doneLabel}
      onHold={() => void confirm()}
      disabled={disabled}
      holdTime={1200}
      releaseTime={220}
      resetAfter={0}
      radius={8}
      wave={false}
      glow={false}
      pressScale={1}
      backgroundColor="var(--color-n800)"
      textColor="var(--color-fg)"
      fillColor="#E23D4F"
      fillTextColor="#FFFFFF"
      className={className}
    >
      {children}
    </HoldButton>
  );
}
