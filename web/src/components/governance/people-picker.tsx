"use client";

import { Search, X } from "lucide-react";
import { useState } from "react";
import { Avatar } from "@/components/ui/avatar";
import { Input } from "@/components/ui/input";
import type { Person } from "./shared";

export function PeoplePicker({
  people,
  value,
  onChange,
  id,
  ...aria
}: {
  people: Person[];
  value: string[];
  onChange: (value: string[]) => void;
  id?: string;
  "aria-describedby"?: string;
  "aria-invalid"?: boolean;
}) {
  const [query, setQuery] = useState("");
  const selected = value.flatMap((personId) => people.filter((p) => p.id === personId));
  const needle = query.trim().toLowerCase();
  const matches = needle ? people.filter((p) => !value.includes(p.id) && `${p.name} ${p.email}`.toLowerCase().includes(needle)).slice(0, 6) : [];

  function add(person: Person) {
    onChange([...value, person.id]);
    setQuery("");
  }

  return (
    <div className="flex flex-col gap-2">
      {selected.length ? (
        <ul className="flex flex-wrap gap-2">
          {selected.map((person) => (
            <li key={person.id} className="inline-flex h-8 items-center gap-2 rounded-md border border-border-strong bg-sunken pr-1 pl-1 text-body-sm text-fg">
              <Avatar name={person.name} size="sm" />
              {person.name}
              <button
                type="button"
                onClick={() => onChange(value.filter((v) => v !== person.id))}
                aria-label={`Remove ${person.name}`}
                className="grid size-6 place-items-center rounded-sm text-fg-3 hover:bg-hover hover:text-fg"
              >
                <X aria-hidden="true" size={16} strokeWidth={1.75} />
              </button>
            </li>
          ))}
        </ul>
      ) : null}
      <Input
        id={id}
        {...aria}
        value={query}
        autoComplete="off"
        onChange={(event) => setQuery(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === "Enter") {
            event.preventDefault();
            if (matches[0]) add(matches[0]);
          }
        }}
        placeholder="Search by name or email"
        leading={<Search size={16} strokeWidth={1.75} />}
      />
      {needle ? (
        matches.length ? (
          <ul className="flex flex-col rounded-md border border-border p-1" aria-label="Matching people">
            {matches.map((person) => (
              <li key={person.id}>
                <button
                  type="button"
                  onClick={() => add(person)}
                  className="flex h-10 w-full items-center gap-3 rounded-sm px-2 text-left text-body-sm text-fg-2 transition-colors duration-fast ease-brand hover:bg-hover hover:text-fg focus-visible:bg-press"
                >
                  <Avatar name={person.name} size="sm" />
                  <span className="truncate text-fg">{person.name}</span>
                  <span className="truncate text-fg-3">{person.email}</span>
                </button>
              </li>
            ))}
          </ul>
        ) : (
          <p className="text-caption text-fg-3">No one in the directory matches “{query.trim()}”.</p>
        )
      ) : null}
    </div>
  );
}
