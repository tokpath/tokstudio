import * as React from "react";
import { cn } from "@/lib/utils";

export const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, type, ...props }, ref) => (
    <input
      type={type}
      className={cn("h-9 rounded-md border border-slate-700 bg-slate-950 px-3 text-sm text-slate-100", className)}
      ref={ref}
      {...props}
    />
  ),
);
Input.displayName = "Input";
