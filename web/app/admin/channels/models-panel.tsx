"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ConfirmButton } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders } from "@/lib/confirm";
import { canWrite, canViewAdminHref } from "@/lib/rbac";
import { useViewer } from "@/components/rbac/viewer-context";
import { millionDim, perTokenToPerMillion } from "@/lib/token-price";

type ChannelModel = {
  public_id: string; display_name: string; vendor: string; kind?: string; status: string; enabled: boolean;
  parent_enabled?: boolean; self_enabled?: boolean; effective_enabled?: boolean; wholesale?: Record<string, string>;
};
type ModelsResponse = { items?: ChannelModel[]; error?: { message?: string } };
type PriceDraft = { input: string; output: string; unit: string };
const unitKey = (model: ChannelModel) => model.kind === "image" ? "image_count" : model.kind === "video" ? "video_second" : "audio_second";
const tokenKind = (model: ChannelModel) => !model.kind || ["text", "embedding"].includes(model.kind);
const draftFor = (model: ChannelModel): PriceDraft => ({ input: perTokenToPerMillion(model.wholesale?.input || ""), output: perTokenToPerMillion(model.wholesale?.output || ""), unit: model.wholesale?.[unitKey(model)] || "" });
const encodePrice = (model: ChannelModel, draft: PriceDraft) => tokenKind(model) ? millionDim(draft.input, model.kind === "embedding" ? "" : draft.output) || {} : { [unitKey(model)]: draft.unit.trim() };

export function ChannelModelsPanel({ channelID, delegated = false }: { channelID: string; delegated?: boolean }) {
  const t = useTranslations("modelGrants");
  const viewer = useViewer();
  const queryClient = useQueryClient();
  const originModel = useSearchParams().get("model") || "";
  const [search, setSearch] = useState(originModel);
  const [filter, setFilter] = useState("all");
  const [granting, setGranting] = useState(false);
  const [snapshot, setSnapshot] = useState<ChannelModel[]>([]);
  const [enabledIDs, setEnabledIDs] = useState<string[]>([]);
  const [prices, setPrices] = useState<Record<string, PriceDraft>>({});
  const [message, setMessage] = useState("");
  const queryKey = [viewer.userId, "/admin/channels", channelID, "models"];
  const query = useQuery({ queryKey, queryFn: async () => {
    const data = await apiClient<ModelsResponse>("GET", `/admin/channels/${encodeURIComponent(channelID)}/models`);
    if (data.error) throw new Error(data.error.message || t("loadError"));
    return data;
  }});
  const items = granting ? snapshot : query.data?.items || [];
  const canGrant = delegated ? viewer.roles.some(role => ["channel_admin", "oem_ops"].includes(role)) : canWrite("models.grant", viewer);
  const selected = useMemo(() => new Set(enabledIDs), [enabledIDs]);
  const changes = items.filter(model => {
    if (!granting) return false;
    const before = draftFor(model), after = prices[model.public_id] || before;
    return model.enabled !== selected.has(model.public_id) || selected.has(model.public_id) && JSON.stringify(before) !== JSON.stringify(after);
  });
  const visible = items.filter(model => `${model.public_id} ${model.display_name} ${model.vendor}`.toLowerCase().includes(search.trim().toLowerCase()) && (filter === "all" || (filter === "enabled" ? model.enabled : !model.enabled)));
  function priceSummary(model: ChannelModel, draft: PriceDraft) { return tokenKind(model) ? `${t("input")}: ${draft.input || "—"}${model.kind === "embedding" ? "" : `; ${t("output")}: ${draft.output || "—"}`}` : `${t(unitKey(model))}: ${draft.unit || "—"}`; }
  function startGrant() {
    const rows = query.data?.items || [];
    setSnapshot(rows);
    setEnabledIDs(rows.filter(model => model.enabled).map(model => model.public_id));
    setPrices(Object.fromEntries(rows.map(model => [model.public_id, draftFor(model)])));
    setGranting(true); setMessage("");
  }
  function validate() {
    for (const model of changes.filter(model => selected.has(model.public_id))) {
      const price = prices[model.public_id] || draftFor(model);
      const required = tokenKind(model) ? model.kind === "embedding" ? [price.input] : [price.input, price.output] : [price.unit];
      if (required.some(value => !/^\d+(?:\.\d+)?$/.test(value.trim()) || !Number.isFinite(Number(value)))) { setMessage(t("invalidPrice", { name: model.display_name })); return false; }
      try { encodePrice(model, price); } catch { setMessage(t("invalidPrice", { name: model.display_name })); return false; }
    }
    setMessage(""); return changes.length > 0;
  }
  async function save() {
    try {
      const response = await fetch(`${apiBase}/admin/channels/${encodeURIComponent(channelID)}/models`, { method: "PATCH", credentials: "include", headers: confirmHeaders,
        body: JSON.stringify({ items: changes.map(model => ({ public_id: model.public_id, enabled: selected.has(model.public_id), ...(selected.has(model.public_id) ? { wholesale: encodePrice(model, prices[model.public_id] || draftFor(model)) } : {}) })) }) });
      const body = await response.json();
      if (!response.ok) { setMessage(body.error?.message || t("saveError")); return false; }
      setGranting(false);
      const result = await query.refetch();
      setMessage(result.isError ? t("savedReadError") : t("saved", { count: changes.length }));
      await queryClient.invalidateQueries({ predicate: query => query.queryKey.some(key => String(key).includes("/admin/oem-deliveries")) });
      return true;
    } catch { setMessage(t("unknownResult")); return false; }
  }
  return <section className="rounded-card border border-hairline bg-canvas-raised p-6">
    <div className="flex flex-wrap items-center justify-between gap-3"><h2 className="text-lg font-semibold">{t("title")}</h2>{canGrant ? granting ? <div className="flex gap-2"><Button variant="outline" onClick={() => setGranting(false)}>{t("cancel")}</Button><ConfirmButton disabled={!changes.length} title={t("confirmTitle")} description={[t("confirmHint"), ...changes.map(model => `${model.display_name} · ${model.public_id}: ${model.enabled ? t("enabled") : t("disabled")} → ${selected.has(model.public_id) ? t("enabled") : t("disabled")}${selected.has(model.public_id) ? `; ${priceSummary(model, draftFor(model))} → ${priceSummary(model, prices[model.public_id] || draftFor(model))}` : ""}`)].join("\n")} validate={validate} onConfirm={save}>{t("save")}</ConfirmButton></div> : <Button disabled={query.isPending || query.isError || !items.length} onClick={startGrant}>{t("edit")}</Button> : null}</div>
    <p className="mt-2 text-sm text-ink-secondary">{t(delegated ? "delegatedHint" : "hint")}</p>
    {originModel && canViewAdminHref("/admin/models", viewer) ? <Link className="mt-3 block text-sm text-brand-emphasis underline" href={`/admin/models/${encodeURIComponent(originModel)}`}>{t("returnModel")}</Link> : null}
    <div className="my-4 flex flex-wrap gap-3"><Input aria-label={t("search")} placeholder={t("search")} value={search} onChange={event => setSearch(event.target.value)} /><select aria-label={t("filter")} className="h-10 rounded-control border border-hairline bg-canvas-raised px-3" value={filter} onChange={event => setFilter(event.target.value)}>{["all", "enabled", "disabled"].map(value => <option key={value} value={value}>{t(value)}</option>)}</select><Button variant="outline" disabled={granting} onClick={() => void query.refetch()}>{t("refresh")}</Button></div>
    {query.isPending ? <p role="status">{t("loading")}</p> : query.isError ? <p role="alert">{t("loadError")}</p> : !visible.length ? <p>{t("empty")}</p> : <ul className="divide-y divide-hairline">{visible.map(model => {
      const checked = granting ? selected.has(model.public_id) : model.enabled;
      const draft = prices[model.public_id] || draftFor(model);
      return <li key={model.public_id} className="py-3"><label className="flex flex-wrap items-center gap-3">{granting ? <input type="checkbox" checked={checked} disabled={!checked && (model.parent_enabled === false || model.status !== "published")} onChange={event => setEnabledIDs(current => event.target.checked ? [...current, model.public_id] : current.filter(id => id !== model.public_id))} /> : null}<span>{model.display_name} <code>{model.public_id}</code></span><span className="text-sm text-ink-secondary">{model.status !== "published" ? t("unpublished") : model.parent_enabled === false ? t("parentDisabled") : checked ? t("enabled") : t("disabled")}</span></label>
        {granting && checked && model.parent_enabled !== false ? <div className="mt-3 grid gap-3 sm:grid-cols-2">{(tokenKind(model) ? model.kind === "embedding" ? ["input"] : ["input", "output"] : ["unit"]).map(key => <label key={key} className="text-sm">{t(key === "unit" ? unitKey(model) : key)}<Input value={draft[key as keyof PriceDraft]} onChange={event => setPrices(current => ({ ...current, [model.public_id]: { ...draft, [key]: event.target.value } }))} /></label>)}</div> : null}
      </li>;
    })}</ul>}
    {message ? <p className="mt-3 text-sm" role="status">{message}</p> : null}
  </section>;
}
