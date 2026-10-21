"use client";
import { useId } from "react";
import { useI18n } from "@/components/i18n";

// A yes/no choice shows both answers at once. A single checkbox whose caption
// flips between 是 and 否 is easy to read backwards when the box is empty.
export function YesNoChoice({ labelledBy, name, value, disabled, onChange }: { labelledBy: string; name: string; value: boolean; disabled?: boolean; onChange: (value: boolean) => void }) {
  const { t } = useI18n();
  const group = useId();
  return <div role="radiogroup" aria-labelledby={labelledBy} className="mt-2 grid grid-cols-2 gap-2">{([[true, "是"], [false, "否"]] as const).map(([next, label]) => <label key={label} className={`flex min-h-11 cursor-pointer items-center gap-3 rounded-control border px-3 py-2 ${value === next ? "border-brand bg-brand-soft/40" : "border-hairline"}`}>
    <input className="size-4 shrink-0" type="radio" name={`${name}-${group}`} checked={value === next} disabled={disabled} onChange={() => onChange(next)} />
    {t(label)}
  </label>)}</div>;
}
