import { LoaderCircle } from "lucide-react";
import { cn } from "@/lib/cn";

export function Spinner({ className, size = 16 }: { className?: string; size?: number }) {
  return (
    <LoaderCircle
      aria-hidden="true"
      size={size}
      strokeWidth={1.75}
      className={cn("animate-spin text-ember", className)}
    />
  );
}
