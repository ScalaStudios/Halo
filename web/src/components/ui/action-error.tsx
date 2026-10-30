import { CircleAlert } from "lucide-react";
import { cn } from "@/lib/cn";

export function ActionError({ message, className }: { message?: string; className?: string }) {
  if (!message) return null;
  return (
    <p role="alert" className={cn("flex items-start gap-2 text-body-sm text-danger", className)}>
      <CircleAlert aria-hidden="true" size={16} strokeWidth={1.75} className="mt-0.5 shrink-0" />
      {message}
    </p>
  );
}
