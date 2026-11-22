"use client";

import { Eye, EyeOff } from "lucide-react";
import { useState } from "react";
import { useMessages } from "@/lib/i18n/client";
import { uiMessages } from "@/lib/i18n/ui";
import { Input, type InputProps } from "./input";

export function PasswordInput({
  visibilityLabels,
  ...props
}: Omit<InputProps, "type" | "trailing"> & { visibilityLabels?: { show: string; hide: string } }) {
  const copy = useMessages(uiMessages);
  const [shown, setShown] = useState(false);
  return (
    <Input
      autoCapitalize="none"
      spellCheck={false}
      {...props}
      type={shown ? "text" : "password"}
      trailing={
        <button
          type="button"
          onClick={() => setShown((current) => !current)}
          aria-label={shown ? (visibilityLabels?.hide ?? copy.hidePassword) : (visibilityLabels?.show ?? copy.showPassword)}
          className="grid size-8 place-items-center rounded-md text-fg-3 transition-colors duration-fast hover:bg-hover hover:text-fg"
        >
          {shown ? <EyeOff aria-hidden="true" size={16} strokeWidth={1.75} /> : <Eye aria-hidden="true" size={16} strokeWidth={1.75} />}
        </button>
      }
    />
  );
}
