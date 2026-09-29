"use client";

import { useState } from "react";
import type { Control, FieldPath, FieldValues } from "react-hook-form";
import { FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { filterProviderOptions, type ProviderOption } from "@/lib/catalog-admin";

export function ProviderSlugCombobox<T extends FieldValues>({
  control,
  name,
  label,
  options,
  placeholder,
  description,
}: {
  control: Control<T>;
  name: FieldPath<T>;
  label: string;
  options: ProviderOption[];
  placeholder?: string;
  description?: string;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const listId = `${String(name)}-list`;

  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => {
        const selected = options.find((item) => item.id === field.value || item.slug === field.value);
        const filtered = filterProviderOptions(options, query);
        return (
          <FormItem>
            <FormLabel>{label}</FormLabel>
            {description ? <p className="text-sm text-ink-secondary">{description}</p> : null}
            <FormControl>
              <div className="relative">
                <Input
                  {...field}
                  role="combobox"
                  aria-label={label}
                  aria-expanded={open}
                  aria-controls={listId}
                  aria-autocomplete="list"
                  autoComplete="off"
                  placeholder={placeholder ?? label}
                  value={open ? query : selected?.name || String(field.value ?? "")}
                  onFocus={() => { setQuery(""); setOpen(true); }}
                  onBlur={() => setOpen(false)}
                  onChange={(event) => {
                    setQuery(event.target.value);
                    field.onChange("");
                    setOpen(true);
                  }}
                />
                {open ? (
                  <ul
                    id={listId}
                    role="listbox"
                    className="absolute z-20 mt-1 max-h-56 w-full overflow-auto rounded-control border border-hairline bg-canvas-raised py-1 shadow-sm"
                  >
                    {filtered.length === 0 ? (
                      <li className="px-3 py-2 text-sm text-ink-secondary">没有匹配的提供商</li>
                    ) : (
                      filtered.map((item) => (
                        <li key={item.id || item.slug} role="option">
                          <button
                            type="button"
                            className="flex w-full items-baseline justify-between gap-3 px-3 py-2 text-left text-sm hover:bg-canvas"
                            onMouseDown={(event) => event.preventDefault()}
                            onClick={() => {
                              field.onChange(item.id);
                              setQuery("");
                              setOpen(false);
                            }}
                          >
                            <span>{item.name || item.slug}</span>
                          </button>
                        </li>
                      ))
                    )}
                  </ul>
                ) : null}
              </div>
            </FormControl>
            <FormMessage />
          </FormItem>
        );
      }}
    />
  );
}
