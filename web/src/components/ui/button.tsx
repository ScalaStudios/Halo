import { forwardRef, type ButtonHTMLAttributes } from "react";
import { cn } from "@/lib/cn";
import { Spinner } from "./spinner";

export type ButtonVariant = "primary" | "secondary" | "tertiary" | "ghost" | "quiet" | "destructive";
export type ButtonSize = "sm" | "md" | "lg" | "icon-sm" | "icon";

const variants: Record<ButtonVariant, string> = {
  primary: "bg-ember text-ink hover:bg-ember-hover hover:shadow-ember active:bg-ember-active active:shadow-none",
  secondary: "border border-border-strong text-fg hover:border-n500 hover:bg-hover active:bg-press",
  tertiary: "text-ember underline-offset-3 hover:text-ember-tint hover:underline active:text-ember-active",
  ghost: "bg-ghost text-ember hover:bg-ghost-hover active:bg-ghost-active",
  quiet: "text-fg-3 hover:bg-hover hover:text-fg active:bg-press data-[state=open]:bg-press data-[state=open]:text-fg",
  destructive: "bg-danger text-white hover:bg-danger-hover active:bg-danger-active",
};

const sizes: Record<ButtonSize, string> = {
  sm: "h-8 px-4 text-body-sm font-semibold",
  md: "h-10 px-6 text-button",
  lg: "h-12 px-8 text-button",
  "icon-sm": "size-8",
  icon: "size-10",
};

export function buttonClasses(variant: ButtonVariant = "secondary", size: ButtonSize = "md", className?: string) {
  return cn(
    "relative inline-flex shrink-0 items-center justify-center gap-2 rounded-md whitespace-nowrap select-none",
    "transition-[background-color,border-color,box-shadow,color] duration-fast ease-brand",
    "disabled:pointer-events-none disabled:opacity-45 aria-disabled:pointer-events-none aria-disabled:opacity-45",
    variants[variant],
    sizes[size],
    className,
  );
}

export type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
};

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = "secondary", size = "md", loading = false, disabled, className, children, type = "button", ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      type={type}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      className={buttonClasses(variant, size, cn(loading && "disabled:opacity-100", className))}
      {...rest}
    >
      <span className={cn("inline-flex items-center gap-2", loading && "invisible")}>{children}</span>
      {loading ? (
        <span className="absolute inset-0 grid place-items-center">
          <Spinner className={variant === "primary" ? "text-ink" : variant === "destructive" ? "text-white" : undefined} />
        </span>
      ) : null}
    </button>
  );
});
