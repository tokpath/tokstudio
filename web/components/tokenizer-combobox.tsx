"use client";

import { catalogLabels } from "@/lib/catalog-copy";
import { useTranslations } from "next-intl";
import { useState } from "react";
import type { Control, FieldPath, FieldValues } from "react-hook-form";
import { FormControl, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import { KNOWN_TOKENIZERS, optionUnion } from "@/lib/catalog";
import { CATALOG_LABEL } from "@/lib/catalog-copy";

export function TokenizerCombobox<T extends FieldValues>({
  control,
  name,
}: {
  control: Control<T>;
  name: FieldPath<T>;
}) {
  const tUI = useTranslations();
  const CATALOG_LABEL = catalogLabels(tUI);
  const [open, setOpen] = useState(false);
  const listId = `${String(name)}-list`;

  return (
    <FormField
      control={control}
      name={name}
      render={({ field }) => {
        const value = String(field.value ?? "");
        const needle = value.trim().toLowerCase();
        const options = optionUnion(KNOWN_TOKENIZERS, [value]).filter((item) =>
          item.toLowerCase().includes(needle),
        );
        return (
          <FormItem>
            <FormLabel>{CATALOG_LABEL.tokenizer}</FormLabel>
            <p className="text-sm text-ink-secondary">{tUI("consoleCopy.usedToEstimateUsageLeaveBlankIfUnsure")}</p>
            <FormControl>
              <div className="relative">
                <Input
                  {...field}
                  role="combobox"
                  aria-label={CATALOG_LABEL.tokenizer}
                  aria-expanded={open}
                  aria-controls={listId}
                  aria-autocomplete="list"
                  autoComplete="off"
                  placeholder={tUI("consoleCopy.enterOrSelectATokenizerOptional")}
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
                    {options.map((item) => {
                      const isKnown = KNOWN_TOKENIZERS.includes(item as (typeof KNOWN_TOKENIZERS)[number]);
                      return (
                        <li key={item} role="option" aria-selected={item === value}>
                          <button
                            type="button"
                            className="flex w-full items-center justify-between gap-3 px-3 py-2 text-left text-sm hover:bg-canvas"
                            onMouseDown={(event) => event.preventDefault()}
                            onClick={() => {
                              field.onChange(item);
                              setOpen(false);
                            }}
                          >
                            <span className="font-mono">{item}</span>
                            {!isKnown ? <span className="text-xs text-ink-secondary">{tUI("consoleCopy.newValue")}</span> : null}
                          </button>
                        </li>
                      );
                    })}
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
