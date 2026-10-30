"use client";

import { Eye, EyeOff } from "lucide-react";
import { useState } from "react";
import { Input, type InputProps } from "./input";

export function PasswordInput(props: Omit<InputProps, "type" | "trailing">) {
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
          aria-label={shown ? "Hide password" : "Show password"}
          className="grid size-8 place-items-center rounded-md text-fg-3 transition-colors duration-fast hover:bg-hover hover:text-fg"
        >
          {shown ? <EyeOff aria-hidden="true" size={16} strokeWidth={1.75} /> : <Eye aria-hidden="true" size={16} strokeWidth={1.75} />}
        </button>
      }
    />
  );
}
