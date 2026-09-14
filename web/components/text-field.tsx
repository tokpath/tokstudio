"use client";

import type { LucideIcon } from "lucide-react";
import type { Control, FieldPath, FieldValues } from "react-hook-form";
import { FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

export function TextField<T extends FieldValues>({
  control,
  name,
  label,
  placeholder,
  type = "text",
  className,
  autoComplete,
  showLabel = true,
  icon: Icon,
  suffix,
}: {
  control: Control<T>;
  name: FieldPath<T>;
  label: string;
  placeholder?: string;
  type?: string;
  className?: string;
  autoComplete?: string;
  showLabel?: boolean;
  icon?: LucideIcon;
  suffix?: string;
}) {
  const ariaLabel = suffix ? `${label}（${suffix}）` : label;
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          {showLabel ? <FormLabel>{label}</FormLabel> : null}
          <FormControl>
            {Icon || suffix ? (
              <div className="relative w-full">
                {Icon ? (
                  <Icon
                    className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-ink-mute"
                    strokeWidth={1.75}
                    aria-hidden
                  />
                ) : null}
                <Input
                  type={type}
                  placeholder={placeholder ?? label}
                  aria-label={ariaLabel}
                  className={cn(Icon && "pl-9", suffix && "pr-16", className)}
                  autoComplete={autoComplete}
                  {...field}
                />
                {suffix ? (
                  <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-xs text-ink-mute">
                    {suffix}
                  </span>
                ) : null}
              </div>
            ) : (
              <Input
                type={type}
                placeholder={placeholder ?? label}
                aria-label={ariaLabel}
                className={className}
                autoComplete={autoComplete}
                {...field}
              />
            )}
          </FormControl>
          <FormMessage />
        </FormItem>
      )}
    />
  );
}
