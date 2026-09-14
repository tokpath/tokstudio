"use client";

import { useState } from "react";
import type { Control, FieldPath, FieldValues } from "react-hook-form";
import { FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { KNOWN_VENDORS, optionUnion, resolveVendorInput, vendorLabel } from "@/lib/catalog";
import { CATALOG_LABEL } from "@/lib/catalog-copy";

export function VendorCombobox<T extends FieldValues>({
  control,
  name,
  extra = [],
  onCommit,
}: {
  control: Control<T>;
  name: FieldPath<T>;
  extra?: string[];
  onCommit?: (vendor: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const listId = `${String(name)}-vendor-list`;

  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => {
        const value = String(field.value ?? "");
        const needle = value.trim().toLowerCase();
        const options = optionUnion(KNOWN_VENDORS, [...extra, value]).filter((item) => {
          const hay = `${item} ${vendorLabel(item)}`.toLowerCase();
          return !needle || hay.includes(needle) || vendorLabel(item).toLowerCase().includes(needle);
        });
        return (
          <FormItem>
            <FormLabel>{CATALOG_LABEL.vendor}</FormLabel>
            <FormControl>
              <div className="relative">
                <Input
                  {...field}
                  role="combobox"
                  aria-label={CATALOG_LABEL.vendor}
                  aria-expanded={open}
                  aria-controls={listId}
                  aria-autocomplete="list"
                  autoComplete="off"
                  placeholder="例如阿里 / OpenAI"
                  onFocus={() => setOpen(true)}
                  onBlur={() => {
                    setOpen(false);
                    field.onBlur();
                    const next = resolveVendorInput(String(field.value ?? ""));
                    if (next !== field.value) {
                      field.onChange(next);
                    }
                    onCommit?.(next);
                  }}
                  onChange={(event) => {
                    field.onChange(event);
                    setOpen(true);
                  }}
                />
                {open ? (
                  <ul
                    id={listId}
                    role="listbox"
                    className="absolute z-20 mt-1 max-h-56 w-full overflow-auto rounded-control border border-hairline bg-canvas-raised py-1 shadow-sm"
                  >
                    {options.map((item) => (
                      <li key={item} role="option" aria-selected={item === value}>
                        <button
                          type="button"
                          className="flex w-full items-baseline justify-between gap-3 px-3 py-2 text-left text-sm hover:bg-canvas"
                          onMouseDown={(event) => event.preventDefault()}
                          onClick={() => {
                            field.onChange(item);
                            setOpen(false);
                            onCommit?.(item);
                          }}
                        >
                          <span>{vendorLabel(item)}</span>
                          <span className="font-mono text-xs text-ink-secondary">{item}</span>
                        </button>
                      </li>
                    ))}
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
