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
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => (
        <FormItem>
          {showLabel ? <FormLabel>{label}</FormLabel> : null}
          <FormControl>
            {Icon ? (
              <div className="relative">
                <Icon
                  className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-ink-mute"
                  strokeWidth={1.75}
                  aria-hidden
                />
                <Input
                  type={type}
                  placeholder={placeholder ?? label}
                  aria-label={label}
                  className={cn("pl-9", className)}
                  autoComplete={autoComplete}
                  {...field}
                />
              </div>
            ) : (
              <Input
                type={type}
                placeholder={placeholder ?? label}
                aria-label={label}
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
