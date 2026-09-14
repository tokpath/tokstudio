"use client";

import { useState } from "react";
import type { Control, FieldPath, FieldValues } from "react-hook-form";
import { FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { CATALOG_LABEL, filterPublicModelOptions, type PublicModelOption } from "@/lib/catalog-copy";

export function PublicModelCombobox<T extends FieldValues>({
  control,
  name,
  options,
}: {
  control: Control<T>;
  name: FieldPath<T>;
  options: PublicModelOption[];
}) {
  const [open, setOpen] = useState(false);
  const listId = `${String(name)}-public-model-list`;

  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => {
        const filtered = filterPublicModelOptions(options, String(field.value ?? ""));
        return (
          <FormItem>
            <FormLabel>{CATALOG_LABEL.publicModelId}</FormLabel>
            <p className="text-sm text-ink-secondary">客户请求里写的名字，不是上游官方模型名。</p>
            <FormControl>
              <div className="relative">
                <Input
                  {...field}
                  role="combobox"
                  aria-label={CATALOG_LABEL.publicModelId}
                  aria-expanded={open}
                  aria-controls={listId}
                  aria-autocomplete="list"
                  autoComplete="off"
                  placeholder="输入显示名或标识筛选"
                  onFocus={() => setOpen(true)}
                  onBlur={() => setOpen(false)}
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
                    {filtered.length === 0 ? (
                      <li className="px-3 py-2 text-sm text-ink-secondary">没有匹配的公开模型</li>
                    ) : (
                      filtered.map((item) => (
                        <li key={item.id} role="option">
                          <button
                            type="button"
                            className="flex w-full items-baseline justify-between gap-3 px-3 py-2 text-left text-sm hover:bg-canvas"
                            onMouseDown={(event) => event.preventDefault()}
                            onClick={() => {
                              field.onChange(item.id);
                              setOpen(false);
                            }}
                          >
                            <span>{item.display_name || item.id}</span>
                            <span className="font-mono text-xs text-ink-secondary">{item.id}</span>
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
