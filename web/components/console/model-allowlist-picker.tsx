"use client";

import { useMemo, useState } from "react";
import { filterPublicModelOptions, type PublicModelOption } from "@/lib/catalog-copy";

export function ModelAllowlistPicker({
  value,
  onChange,
  options,
  allLabel,
  searchLabel,
  selectedLabel,
}: {
  value?: string[];
  onChange: (next: string[]) => void;
  options: PublicModelOption[];
  allLabel: string;
  searchLabel: string;
  selectedLabel: string;
}) {
  const [query, setQuery] = useState("");
  const selectedIds = value ?? [];
  const selected = new Set(selectedIds);
  const filtered = useMemo(() => filterPublicModelOptions(options, query, 40), [options, query]);

  function toggle(id: string) {
    if (selected.has(id)) {
      onChange(selectedIds.filter((item) => item !== id));
      return;
    }
    onChange([...selectedIds, id]);
  }

  return (
    <div className="space-y-2">
      {selectedIds.length === 0 ? (
        <p className="text-sm text-ink-secondary">{allLabel}</p>
      ) : (
        <ul className="flex flex-wrap gap-1.5" aria-label={selectedLabel}>
          {selectedIds.map((id) => (
            <li key={id}>
              <button
                type="button"
                className="rounded-control border border-hairline bg-canvas px-2 py-1 font-mono text-xs text-ink"
                onClick={() => toggle(id)}
              >
                {id} ×
              </button>
            </li>
          ))}
        </ul>
      )}
      <input
        type="search"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        aria-label={searchLabel}
        placeholder={searchLabel}
        className="h-10 w-full rounded-control border border-hairline bg-canvas px-3 text-sm text-ink"
      />
      <ul className="max-h-40 overflow-auto rounded-control border border-hairline bg-canvas-raised py-1">
        {filtered.length === 0 ? (
          <li className="px-3 py-2 text-sm text-ink-secondary">{searchLabel}</li>
        ) : (
          filtered.map((item) => (
            <li key={item.id}>
              <label className="flex cursor-pointer items-baseline justify-between gap-3 px-3 py-2 text-sm hover:bg-canvas">
                <span className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    className="accent-brand"
                    checked={selected.has(item.id)}
                    onChange={() => toggle(item.id)}
                  />
                  <span>{item.display_name || item.id}</span>
                </span>
                <span className="font-mono text-xs text-ink-secondary">{item.id}</span>
              </label>
            </li>
          ))
        )}
      </ul>
    </div>
  );
}
