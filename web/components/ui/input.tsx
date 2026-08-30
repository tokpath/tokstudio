import * as React from "react";
import { cn } from "@/lib/utils";

export const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, type, ...props }, ref) => (
    <input
      type={type}
      className={cn(
        "h-10 rounded-lg border border-white/10 bg-black/30 px-3 text-sm text-slate-100 placeholder:text-slate-500 focus:border-[color-mix(in_srgb,var(--brand-primary)_50%,transparent)]",
        className,
      )}
      ref={ref}
      {...props}
    />
  ),
);
Input.displayName = "Input";
