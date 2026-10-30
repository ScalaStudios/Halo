"use client";

import type { ReactNode } from "react";
import HoldButton from "@/components/reactbits/HoldButton";

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
  onConfirm: () => void;
  size?: "sm" | "md";
  disabled?: boolean;
  className?: string;
}) {
  return (
    <HoldButton
      size={size}
      icon={icon}
      doneLabel={doneLabel}
      onHold={onConfirm}
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
