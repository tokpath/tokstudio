import * as React from "react";
import { cn } from "@/lib/utils";

export const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, type, ...props }, ref) => (
    <input
      type={type}
      className={cn(
        "h-10 min-h-10 rounded-control border border-hairline bg-canvas-raised px-3 text-sm text-ink placeholder:text-ink-mute focus:border-brand-emphasis max-sm:min-h-11",
        className,
      )}
      ref={ref}
      {...props}
    />
  ),
);
Input.displayName = "Input";
