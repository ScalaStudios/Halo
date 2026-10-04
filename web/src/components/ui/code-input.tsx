"use client";

import { useEffect, useState } from "react";
import CodeSlots, { type CodeSlotsStatus } from "@/components/reactbits/CodeSlots";

export function CodeInput({
  value,
  onChange,
  onComplete,
  status = "idle",
  disabled,
  autoFocus,
  label = "Six-digit code",
}: {
  value: string;
  onChange: (code: string) => void;
  onComplete?: (code: string) => void;
  status?: CodeSlotsStatus;
  disabled?: boolean;
  autoFocus?: boolean;
  label?: string;
}) {
  const [compact, setCompact] = useState(false);

  useEffect(() => {
    const query = window.matchMedia("(max-width: 419px)");
    const update = () => setCompact(query.matches);
    update();
    query.addEventListener("change", update);
    return () => query.removeEventListener("change", update);
  }, []);

  return (
    <CodeSlots
      length={6}
      value={value}
      onChange={onChange}
      onComplete={onComplete}
      status={status}
      disabled={disabled}
      autoFocus={autoFocus}
      ariaLabel={label}
      slotSize={compact ? 36 : 48}
      gap={8}
      radius={8}
      bounce={0}
      settle={0.22}
      rise={4}
      cascade={24}
      slotColor="#0C0B0A"
      accentColor="#2A2724"
      digitColor="#FFFFFF"
      inkColor="#FA7E26"
      dangerColor="#E23D4F"
      className="font-mono"
    />
  );
}
