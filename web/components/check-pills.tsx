"use client";

import type { Control, FieldPath, FieldValues } from "react-hook-form";
import { FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { cn } from "@/lib/utils";

export function CheckPills<T extends FieldValues>({
  control,
  name,
  label,
  options,
  hint,
}: {
  control: Control<T>;
  name: FieldPath<T>;
  label: string;
  options: string[];
  hint?: string;
}) {
  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => {
        const selected = Array.isArray(field.value) ? (field.value as string[]) : [];
        return (
          <FormItem>
            <FormLabel>{label}</FormLabel>
            {hint ? <p className="text-sm text-ink-secondary">{hint}</p> : null}
            <FormControl>
              <div className="flex flex-wrap gap-2" role="group" aria-label={label}>
                {options.map((option) => {
                  const on = selected.includes(option);
                  return (
                    <button
                      key={option}
                      type="button"
                      aria-pressed={on}
                      className={cn(
                        "rounded-control border px-2.5 py-1 font-mono text-xs leading-6 transition-colors",
                        on
                          ? "border-brand-emphasis bg-brand/10 text-ink"
                          : "border-hairline bg-transparent text-ink-secondary hover:bg-canvas",
                      )}
                      onClick={() => {
                        field.onChange(on ? selected.filter((item) => item !== option) : [...selected, option]);
                      }}
                    >
                      {option}
                    </button>
                  );
                })}
              </div>
            </FormControl>
            <FormMessage />
          </FormItem>
        );
      }}
    />
  );
}
