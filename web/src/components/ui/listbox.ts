"use client";

import { useId, useState, type KeyboardEvent } from "react";

export function useListbox<T>(options: T[], key: (option: T) => string, pick: (option: T) => void) {
  const listId = useId();
  const [active, setActive] = useState("");
  const highlighted = options.find((option) => key(option) === active) ?? options[0];
  const optionId = (option: T) => `${listId}-${key(option)}`;

  function onKeyDown(event: KeyboardEvent) {
    if (!highlighted || event.nativeEvent.isComposing) return;
    const step = event.key === "ArrowDown" ? 1 : event.key === "ArrowUp" ? -1 : 0;
    if (step) {
      event.preventDefault();
      const next = options.at((options.indexOf(highlighted) + step) % options.length) ?? highlighted;
      setActive(key(next));
      document.getElementById(optionId(next))?.scrollIntoView({ block: "nearest" });
    } else if (event.key === "Enter") {
      event.preventDefault();
      pick(highlighted);
    }
  }

  return {
    listId,
    optionId,
    highlighted,
    activeId: highlighted && optionId(highlighted),
    highlight: (option: T) => setActive(key(option)),
    reset: () => setActive(""),
    onKeyDown,
  };
}
