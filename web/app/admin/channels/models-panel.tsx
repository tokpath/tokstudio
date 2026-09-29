"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ConfirmButton } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { canWrite } from "@/lib/rbac";
import { useViewer } from "@/components/rbac/viewer-context";
import { millionDim, perTokenToPerMillion } from "@/lib/token-price";

type ChannelModel = {
  public_id: string; display_name: string; vendor: string; kind?: string; status: string; enabled: boolean;
  wholesale?: Record<string, string>; customer_override?: Record<string, string>;
};
type ModelsResponse = { items?: ChannelModel[]; error?: { message?: string } };
type PriceDraft = { wholesale_input: string; wholesale_output: string; wholesale_unit: string;
  customer_input: string; customer_output: string; customer_unit: string };

function priceDraft(model: ChannelModel): PriceDraft {
  const kind = model.kind || "text";
  const unit = kind === "image" ? "image_count" : kind === "video" ? "video_second" : "audio_second";
  return {
    wholesale_input: perTokenToPerMillion(model.wholesale?.input || ""),
    wholesale_output: perTokenToPerMillion(model.wholesale?.output || ""),
    wholesale_unit: model.wholesale?.[unit] || "",
    customer_input: perTokenToPerMillion(model.customer_override?.input || ""),
    customer_output: perTokenToPerMillion(model.customer_override?.output || ""),
    customer_unit: model.customer_override?.[unit] || "",
  };
}

function priceValues(model: ChannelModel, draft: PriceDraft) {
  const kind = model.kind || "text";
  if (kind === "text" || kind === "embedding") {
    return { wholesale: millionDim(draft.wholesale_input, draft.wholesale_output) || {},
      customer_override: millionDim(draft.customer_input, draft.customer_output) || {} };
  }
  const key = kind === "image" ? "image_count" : kind === "video" ? "video_second" : "audio_second";
  return { wholesale: draft.wholesale_unit.trim() ? { [key]: draft.wholesale_unit.trim() } : {},
    customer_override: draft.customer_unit.trim() ? { [key]: draft.customer_unit.trim() } : {} };
}

export function ChannelModelsPanel({ channelID }: { channelID: string }) {
  const queryClient = useQueryClient();
  const [granting, setGranting] = useState(false);
  const [enabledIDs, setEnabledIDs] = useState<string[]>([]);
  const [prices, setPrices] = useState<Record<string, PriceDraft>>({});
  const [message, setMessage] = useState("这里只授权已发布且路由已启用的模型。");
  const viewer = useViewer();
  const modelsQuery = useQuery({
    queryKey: ["/admin/channels", channelID, "models"],
    queryFn: () => apiClient<ModelsResponse>("GET", `/admin/channels/${channelID}/models`),
  });
  const items = modelsQuery.data?.items ?? [];
  const canGrant = canWrite("models.grant", viewer);
  const selected = useMemo(() => new Set(enabledIDs), [enabledIDs]);

  function startGrant() {
    setEnabledIDs(items.filter((item) => item.enabled).map((item) => item.public_id));
    setPrices(Object.fromEntries(items.map((item) => [item.public_id, priceDraft(item)])));
    setGranting(true);
  }

  function updatePrice(id: string, key: keyof PriceDraft, value: string) {
    setPrices((current) => ({ ...current, [id]: { ...current[id], [key]: value } }));
  }

  function validatePrices() {
    const valid = (value: string) => /^\d+(?:\.\d+)?$/.test(value.trim()) && Number.isFinite(Number(value)) && Number(value) >= 0;
    for (const model of items) {
      if (!selected.has(model.public_id)) continue;
      const draft = prices[model.public_id] || priceDraft(model);
      const kind = model.kind || "text";
      if ((kind === "text" && (!draft.wholesale_input.trim() || !draft.wholesale_output.trim())) ||
          (kind === "embedding" && !draft.wholesale_input.trim()) ||
          (!["text", "embedding"].includes(kind) && !draft.wholesale_unit.trim())) {
        setMessage(`请填写 ${model.display_name} 的渠道结算价`);
        return false;
      }
      const required = kind === "text" ? [draft.wholesale_input, draft.wholesale_output] :
        kind === "embedding" ? [draft.wholesale_input] : [draft.wholesale_unit];
      const optional = kind === "text" ? [draft.customer_input, draft.customer_output] :
        kind === "embedding" ? [draft.customer_input] : [draft.customer_unit];
      if (required.some((value) => !valid(value)) || optional.some((value) => value.trim() && !valid(value))) {
        setMessage(`请检查 ${model.display_name} 的价格，须填写非负数字`);
        return false;
      }
      if (kind === "text" && optional.some((value) => value.trim()) && optional.some((value) => !value.trim())) {
        setMessage(`请完整填写 ${model.display_name} 的此渠道客户输入价和输出价`);
        return false;
      }
      try { priceValues(model, draft); } catch (error) {
        setMessage(error instanceof Error ? error.message : "价格格式无效");
        return false;
      }
    }
    return true;
  }

  return (
    <section className="rounded-stamp border border-hairline bg-canvas-raised p-6">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <h3 className="text-lg font-medium">平台模型白名单</h3>
        {canGrant && granting ? (
          <div className="flex flex-wrap gap-2">
            <Button size="sm" variant="outline" onClick={() => setGranting(false)}>
              取消
            </Button>
            <ConfirmButton
              size="sm"
              title="确认授权平台模型"
              description="保存模型授权及渠道价格；只影响之后的请求。"
              validate={validatePrices}
              onConfirm={async () => {
                    try {
                const res = await fetch(`${apiBase}/admin/channels/${channelID}/models`, {
                  method: "PATCH",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify({
                    items: items.map((item) => ({
                      public_id: item.public_id, enabled: selected.has(item.public_id),
                      ...(selected.has(item.public_id) ? priceValues(item, prices[item.public_id] || priceDraft(item)) : {}),
                    })),
                  }),
                });
                const body = await res.json();
                if (!res.ok) {
                  setMessage(body.error?.message || "保存失败");
                  return false;
                }
                setMessage(`已按平台目录更新白名单，共 ${body.items?.length ?? 0} 个模型`);
                setGranting(false);
                await queryClient.invalidateQueries({ queryKey: ["/admin/channels", channelID, "models"] });
                    return true;
                    } catch {
                      setMessage(confirmNetworkUnavailable);
                      return false;
                    }
}}
            >
              保存白名单
            </ConfirmButton>
          </div>
        ) : canGrant ? (
          <Button size="sm" onClick={startGrant}>
            从平台目录授权
          </Button>
        ) : null}
      </div>
      <p className="mb-3 text-sm text-ink-secondary">
        租户不能自己添加提供商和模型。这里只有已发布且路由已启用的模型可新增授权。要新增模型请到平台
        <Link href="/admin/models" className="mx-1 text-brand-emphasis no-underline hover:underline">
          模型
        </Link>
        或
        <Link href="/admin/providers" className="mx-1 text-brand-emphasis no-underline hover:underline">
          提供商
        </Link>
        页，接入上游请到路由组。
      </p>
      {modelsQuery.data?.error ? (
        <p className="text-sm text-ink-secondary">{modelsQuery.data.error.message}</p>
      ) : items.length === 0 ? (
        <p className="text-sm text-ink-secondary">登录平台管理员后可以看到目录授权。</p>
      ) : (
        <ul className="grid gap-2 text-sm">
          {items.map((model) => {
            const checked = granting ? selected.has(model.public_id) : model.enabled;
            return (
              <li key={model.public_id} className="border-b border-hairline py-2">
                <div className="flex flex-wrap items-center justify-between gap-2">
                <label className="flex min-w-0 items-center gap-2">
                  {granting ? (
                    <input
                      type="checkbox"
                      aria-label={`授权 ${model.public_id}`}
                      checked={checked}
                      onChange={(event) => {
                        setEnabledIDs((current) => {
                          const next = new Set(current);
                          if (event.target.checked) {
                            next.add(model.public_id);
                          } else {
                            next.delete(model.public_id);
                          }
                          return Array.from(next);
                        });
                      }}
                    />
                  ) : null}
                  <span>
                    {model.display_name} <span className="font-mono text-ink-secondary">{model.public_id}</span>
                  </span>
                </label>
                <span className="text-ink-secondary">
                  {model.vendor} · {model.status} · {checked ? "已授权" : "未授权"}
                </span>
                </div>
                {granting && checked ? <div className="mt-3 grid gap-3 pl-6 sm:grid-cols-2">
                  {(model.kind || "text") === "text" || model.kind === "embedding" ? <>
                    <label>渠道结算价 输入（美元/M）<input className="h-10 w-full rounded-control border border-hairline px-3" value={prices[model.public_id]?.wholesale_input || ""} onChange={(event) => updatePrice(model.public_id, "wholesale_input", event.target.value)} /></label>
                    {(model.kind || "text") === "text" ? <label>渠道结算价 输出（美元/M）<input className="h-10 w-full rounded-control border border-hairline px-3" value={prices[model.public_id]?.wholesale_output || ""} onChange={(event) => updatePrice(model.public_id, "wholesale_output", event.target.value)} /></label> : null}
                    <label>此渠道客户价 输入（可选，美元/M）<input className="h-10 w-full rounded-control border border-hairline px-3" value={prices[model.public_id]?.customer_input || ""} onChange={(event) => updatePrice(model.public_id, "customer_input", event.target.value)} /></label>
                    {(model.kind || "text") === "text" ? <label>此渠道客户价 输出（可选，美元/M）<input className="h-10 w-full rounded-control border border-hairline px-3" value={prices[model.public_id]?.customer_output || ""} onChange={(event) => updatePrice(model.public_id, "customer_output", event.target.value)} /></label> : null}
                  </> : <>
                    <label>渠道结算价（美元/单位）<input className="h-10 w-full rounded-control border border-hairline px-3" value={prices[model.public_id]?.wholesale_unit || ""} onChange={(event) => updatePrice(model.public_id, "wholesale_unit", event.target.value)} /></label>
                    <label>此渠道客户价（可选，美元/单位）<input className="h-10 w-full rounded-control border border-hairline px-3" value={prices[model.public_id]?.customer_unit || ""} onChange={(event) => updatePrice(model.public_id, "customer_unit", event.target.value)} /></label>
                  </>}
                </div> : null}
              </li>
            );
          })}
        </ul>
      )}
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </section>
  );
}
