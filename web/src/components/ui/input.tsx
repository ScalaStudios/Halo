import { forwardRef, useId, type ComponentProps, type InputHTMLAttributes, type ReactNode, type SelectHTMLAttributes, type TextareaHTMLAttributes } from "react";
import { ChevronDown } from "lucide-react";
import { cn } from "@/lib/cn";

const control = cn(
  "w-full rounded-md border border-border-strong bg-sunken text-fg",
  "transition-[border-color,box-shadow] duration-fast ease-brand",
  "hover:border-n500 focus-visible:border-ember disabled:opacity-45",
  "aria-invalid:border-danger aria-invalid:focus-visible:border-danger",
);

const sizes = {
  sm: "h-8 px-3 text-body-sm",
  md: "h-10 px-4 text-body",
} as const;

export type InputProps = Omit<InputHTMLAttributes<HTMLInputElement>, "size"> & {
  size?: keyof typeof sizes;
  mono?: boolean;
  leading?: ReactNode;
  trailing?: ReactNode;
};

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { className, size = "md", mono = false, leading, trailing, ...rest },
  ref,
) {
  const input = (
    <input
      ref={ref}
      className={cn(control, sizes[size], mono && "font-mono text-code-sm", leading ? "pl-9" : null, trailing ? "pr-12" : null, className)}
      {...rest}
    />
  );
  if (!leading && !trailing) return input;
  return (
    <div className="relative w-full">
      {leading ? (
        <span className="pointer-events-none absolute top-1/2 left-3 grid -translate-y-1/2 place-items-center text-fg-3">{leading}</span>
      ) : null}
      {input}
      {trailing ? <span className="absolute top-1/2 right-2 flex -translate-y-1/2 items-center">{trailing}</span> : null}
    </div>
  );
});

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaHTMLAttributes<HTMLTextAreaElement> & { mono?: boolean }>(
  function Textarea({ className, rows = 3, mono = false, ...rest }, ref) {
    return (
      <textarea
        ref={ref}
        rows={rows}
        className={cn(control, "px-4 py-3 text-body", mono && "font-mono text-code-sm", className)}
        {...rest}
      />
    );
  },
);

export const Select = forwardRef<
  HTMLSelectElement,
  Omit<SelectHTMLAttributes<HTMLSelectElement>, "size"> & { size?: keyof typeof sizes }
>(function Select({ className, size = "md", children, ...rest }, ref) {
  return (
    <div className="relative w-full">
      <select ref={ref} className={cn(control, sizes[size], "appearance-none pr-10", className)} {...rest}>
        {children}
      </select>
      <ChevronDown
        aria-hidden="true"
        size={16}
        strokeWidth={1.75}
        className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-fg-3"
      />
    </div>
  );
});

export function Checkbox({ className, ...rest }: Omit<ComponentProps<"input">, "type">) {
  return (
    <input
      type="checkbox"
      className={cn(
        "halo-checkbox size-4 shrink-0 cursor-pointer appearance-none rounded-sm border border-border-strong bg-sunken bg-center bg-no-repeat",
        "transition-[background-color,border-color] duration-fast ease-brand hover:border-n500",
        "checked:border-ember checked:bg-ember indeterminate:border-ember indeterminate:bg-ember",
        className,
      )}
      {...rest}
    />
  );
}

export function Field({
  label,
  hint,
  error,
  children,
  id,
  className,
}: {
  label: ReactNode;
  hint?: ReactNode;
  error?: ReactNode;
  children: (props: { id: string; "aria-describedby"?: string; "aria-invalid"?: boolean }) => ReactNode;
  id?: string;
  className?: string;
}) {
  const generated = useId();
  const fieldId = id ?? generated;
  const describedBy = error ? `${fieldId}-error` : hint ? `${fieldId}-hint` : undefined;
  return (
    <div className={cn("flex flex-col gap-2", className)}>
      <label htmlFor={fieldId} className="text-label text-fg">
        {label}
      </label>
      {children({ id: fieldId, "aria-describedby": describedBy, "aria-invalid": error ? true : undefined })}
      {error ? (
        <p id={`${fieldId}-error`} className="text-caption text-danger">
          {error}
        </p>
      ) : hint ? (
        <p id={`${fieldId}-hint`} className="text-caption text-fg-3">
          {hint}
        </p>
      ) : null}
    </div>
  );
}
