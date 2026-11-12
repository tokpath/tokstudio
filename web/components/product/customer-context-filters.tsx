"use client";

import { useState } from "react";
import { Search } from "lucide-react";
import { useI18n } from "@/components/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useMonetary } from "./monetary";
import { resourceFilterStatusLabel } from "./operator-resources";
import { AlertText } from "./shared";

export type CustomerContextFilterValues = { q: string; status: string; unit: string; from: string; to: string };
export type CustomerContextFilterChoices = { statuses?: string[]; units?: string[] };
export const emptyCustomerContextFilters: CustomerContextFilterValues = { q: "", status: "", unit: "", from: "", to: "" };

function filterQuery(values: CustomerContextFilterValues) {
  // Match the local calendar shown in the date fields. The API uses an
  // exclusive upper bound; adding a calendar day also covers DST changes.
  const start = values.from ? new Date(`${values.from}T00:00:00`) : undefined;
  const end = values.to ? new Date(`${values.to}T00:00:00`) : undefined;
  if (end) end.setDate(end.getDate() + 1);
  const validBound = (date?: Date) => !date || (Number.isFinite(date.getTime()) && date.getUTCFullYear() <= 9999 && date.getUTCFullYear() > 0);
  if (!validBound(start) || !validBound(end) || (start && end && start >= end)) return;
  const params = new URLSearchParams();
  if (values.q) params.set("q", values.q);
  if (values.status) params.set("status", values.status);
  if (values.unit) params.set("unit", values.unit);
  if (start) params.set("from", start.toISOString());
  if (end) params.set("to", end.toISOString());
  return params.toString();
}

export function CustomerContextFilters({ resource, choices, value, disabled, fetching, onApply }: {
  resource: string;
  choices: CustomerContextFilterChoices;
  value: CustomerContextFilterValues;
  disabled: boolean;
  fetching: boolean;
  onApply: (value: CustomerContextFilterValues, query: string) => void;
}) {
  const { t } = useI18n();
  const { currency } = useMonetary();
  const [draft, setDraft] = useState(value);
  const [invalidDate, setInvalidDate] = useState(false);
  const change = (key: keyof CustomerContextFilterValues, text: string) => {
    setDraft(current => ({ ...current, [key]: text }));
    if (key === "from" || key === "to") setInvalidDate(false);
  };
  const selectClass = "block min-h-11 w-full max-w-full rounded-control border border-hairline bg-canvas-raised px-3 text-sm text-ink disabled:opacity-50";
  return <form className="mb-5" onSubmit={event => {
    event.preventDefault();
    if (disabled) return;
    const next = { ...draft, q: draft.q.trim() };
    const query = filterQuery(next);
    setInvalidDate(query === undefined);
    if (query !== undefined) {
      setDraft(next);
      onApply(next, query);
    }
  }}>
    <fieldset disabled={disabled} className="flex min-w-0 flex-wrap items-end gap-2">
      <div className="relative min-w-0 flex-1 basis-64">
        <Search size={15} aria-hidden className="absolute left-3 top-3.5 text-ink-mute" />
        <Input type="search" aria-label={t("搜索记录")} placeholder={t("搜索业务编号、名称或关联记录")} className="pl-9" maxLength={200} value={draft.q} onChange={event => change("q", event.target.value)} />
      </div>
      {!!choices.statuses?.length && <label className="min-w-0 max-w-full space-y-1 text-xs text-ink-secondary">
        <span>{t("状态")}</span>
        <select className={selectClass} aria-label={t("筛选状态")} value={draft.status} onChange={event => change("status", event.target.value)}>
          <option value="">{t("全部状态")}</option>
          {choices.statuses.map(status => <option key={status} value={status}>{t(resourceFilterStatusLabel(resource, status))}</option>)}
        </select>
      </label>}
      {!!choices.units?.length && <label className="min-w-0 max-w-full space-y-1 text-xs text-ink-secondary">
        <span>{t("单位")}</span>
        <select className={selectClass} value={draft.unit} onChange={event => change("unit", event.target.value)}>
          <option value="">{t("全部")}</option>
          {choices.units.map(unit => <option key={unit} value={unit}>{unit === "MONEY" ? currency : unit}</option>)}
        </select>
      </label>}
      <label className="min-w-0 space-y-1 text-xs text-ink-secondary">
        <span>{t("开始日期")}</span>
        <Input type="date" value={draft.from} onChange={event => change("from", event.target.value)} />
      </label>
      <label className="min-w-0 space-y-1 text-xs text-ink-secondary">
        <span>{t("结束日期")}</span>
        <Input type="date" value={draft.to} onChange={event => change("to", event.target.value)} />
      </label>
      <Button variant="outline" type="submit" aria-busy={fetching}>{t("查询")}</Button>
      {(Object.values(draft).some(Boolean) || Object.values(value).some(Boolean)) && <Button variant="ghost" type="button" onClick={() => {
        setDraft(emptyCustomerContextFilters);
        setInvalidDate(false);
        onApply(emptyCustomerContextFilters, "");
      }}>{t("清除")}</Button>}
    </fieldset>
    {invalidDate && <AlertText className="mt-2 text-sm text-danger">{t("请选择有效日期范围。")}</AlertText>}
  </form>;
}
