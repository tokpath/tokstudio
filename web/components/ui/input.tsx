import * as React from "react";
import { cn } from "@/lib/utils";

export const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, type, ...props }, ref) => (
    <input
      type={type}
      className={cn(
        "h-10 min-h-10 w-full rounded-control border border-hairline bg-canvas-raised px-3 py-2 text-sm leading-normal text-ink placeholder:text-ink-mute transition-colors duration-150 ease-out focus:border-brand-emphasis max-sm:min-h-11",
        className,
      )}
      ref={ref}
      {...props}
    />
  ),
);
Input.displayName = "Input";
