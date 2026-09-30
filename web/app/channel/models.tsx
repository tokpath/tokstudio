"use client";

import { canChannelAction } from "@/lib/rbac";
import { useState } from "react";
import { ConfirmButton } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useViewer } from "@/components/rbac/viewer-context";
import { ListResourceView } from "@/components/console/list-resource-view";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { fetchListItems } from "@/lib/list-resource";
import { millionDim, perTokenToPerMillion } from "@/lib/token-price";

type ChannelModel = { public_id: string; display_name?: string; vendor?: string; kind?: string; status?: string; enabled?: boolean; self_enabled?: boolean; parent_enabled?: boolean; effective_enabled?: boolean; customer_override?: Record<string, string> };

export default function ChannelModels() {
  const [message, setMessage] = useState("");
  const [editingPrice, setEditingPrice] = useState("");
  const [priceInput, setPriceInput] = useState("");
  const [priceOutput, setPriceOutput] = useState("");
  const [priceUnit, setPriceUnit] = useState("");
  const viewer = useViewer();
  const list = useListResource<ChannelModel>({
    load: () => fetchListItems(`${apiBase}/channel/models`),
  });

  async function changeOwnStatus(item: ChannelModel, enabled: boolean): Promise<boolean> {
    try {
      const res = await fetch(`${apiBase}/channel/models`, {
        method: "PATCH", credentials: "include", headers: confirmHeaders,
        body: JSON.stringify({ public_id: item.public_id, enabled }),
      });
      const body = await res.json();
      setMessage(res.ok ? `已${enabled ? "上架" : "下架"} ${item.display_name || item.public_id}` : body.error?.message || "操作失败");
      if (res.ok) await list.reload();
      return res.ok;
    } catch {
      setMessage(confirmNetworkUnavailable);
      return false;
    }
  }

  function startPrice(item: ChannelModel) {
    const unit = item.kind === "image" ? "image_count" : item.kind === "video" ? "video_second" : "audio_second";
    setEditingPrice(item.public_id);
    setPriceInput(perTokenToPerMillion(item.customer_override?.input || ""));
    setPriceOutput(perTokenToPerMillion(item.customer_override?.output || ""));
    setPriceUnit(item.customer_override?.[unit] || "");
    setMessage("");
  }

  async function savePrice(item: ChannelModel): Promise<boolean> {
    const kind = item.kind || "text";
    const unit = kind === "image" ? "image_count" : kind === "video" ? "video_second" : "audio_second";
    const textPrice = kind === "text" || kind === "embedding";
    const valid = (value: string) => /^\d+(?:\.\d+)?$/.test(value.trim()) && Number(value) >= 0;
    if (textPrice ? ((priceInput || priceOutput) && (!valid(priceInput) || (kind === "text" && !valid(priceOutput)))) : (priceUnit && !valid(priceUnit))) {
      setMessage("请输入完整的非负品牌客户价，或清空以沿用公开价。");
      return false;
    }
    let prices: Record<string, string> = {};
    try {
      prices = textPrice ? (priceInput ? millionDim(priceInput, kind === "text" ? priceOutput : "") || {} : {}) : (priceUnit ? { [unit]: priceUnit.trim() } : {});
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "价格格式无效");
      return false;
    }
    try {
      const response = await fetch(`${apiBase}/channel/model-prices`, {
        method: "PATCH", credentials: "include", headers: confirmHeaders,
        body: JSON.stringify({ public_id: item.public_id, customer_price: prices }),
      });
      const body = await response.json();
      setMessage(response.ok ? `已更新 ${item.display_name || item.public_id} 的品牌客户价` : body.error?.message || "保存失败");
      if (response.ok) {
        setEditingPrice("");
        await list.reload();
      }
      return response.ok;
    } catch {
      setMessage(confirmNetworkUnavailable);
      return false;
    }
  }

  return (
    <Card>
      <CardTitle className="mb-4 text-lg font-semibold tracking-tight">本渠道模型</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">
        可下架本渠道模型；下属渠道也会立即停用。{viewer.channelType === "C" ? "品牌客户价在这里统一设置，下属渠道自动继承。" : "客户价由所属品牌统一设置。"}
      </p>
      <Button variant="outline" onClick={() => void list.reload()}>
        刷新模型
      </Button>
      {message ? <p role="status" className="mt-2 text-sm text-ink-secondary">{message}</p> : null}
      <ListResourceView
        snapshot={list.snapshot}
        emptyTitle="暂无已授权模型"
        emptyDetail="平台还没有给本渠道授权模型。开通后会出现在这里。"
        onRetry={() => void list.reload()}
      >
        <ul className="mt-4 grid gap-2 text-sm">
          {list.snapshot.items.map((item) => (
          <li key={item.public_id} className="flex flex-wrap justify-between gap-2 border-b border-hairline py-2">
              <span>
                {item.display_name || item.public_id} <span className="font-mono text-ink-secondary">{item.public_id}</span>
              </span>
            <span className="text-ink-secondary">
                {item.vendor} · {item.status !== "published" ? "平台已下架" : !item.enabled ? "上级已取消授权" : item.parent_enabled === false ? "上级已下架" : item.self_enabled === false ? "本渠道已下架" : "可使用"}
            </span>
            {item.enabled && canChannelAction("operations", viewer) ? <ConfirmButton
              size="sm" variant="outline"
              disabled={item.status !== "published" || (item.parent_enabled === false && item.self_enabled === false)}
              title={item.self_enabled === false ? "确认上架模型" : "确认下架模型"}
              description={item.self_enabled === false ? "恢复本渠道及已获授权下属渠道的使用。" : "本渠道及已获授权下属渠道会立即停用此模型。"}
              onConfirm={() => changeOwnStatus(item, item.self_enabled === false)}
            >{item.self_enabled === false ? "上架" : "下架"}</ConfirmButton> : null}
            {viewer.channelType === "C" && item.enabled && canChannelAction("finance", viewer) ? <Button size="sm" variant="outline" onClick={() => startPrice(item)}>品牌客户价</Button> : null}
            {editingPrice === item.public_id ? <div className="mt-2 grid w-full gap-2 rounded-control border border-hairline p-3">
              {item.kind === "image" || item.kind === "video" || item.kind === "audio" ? <label className="grid gap-1">客户价（美元/单位）<Input aria-label="品牌客户单位价" value={priceUnit} onChange={(event) => setPriceUnit(event.target.value)} /></label> : <>
                <label className="grid gap-1">输入（美元/M）<Input aria-label="品牌客户输入价" value={priceInput} onChange={(event) => setPriceInput(event.target.value)} /></label>
                {item.kind !== "embedding" ? <label className="grid gap-1">输出（美元/M）<Input aria-label="品牌客户输出价" value={priceOutput} onChange={(event) => setPriceOutput(event.target.value)} /></label> : null}
              </>}
              <p className="text-xs text-ink-secondary">留空则沿用平台公开价。修改后本品牌及下属渠道的新请求立即使用此价格。</p>
              <div className="flex gap-2"><ConfirmButton size="sm" title="确认更新品牌客户价" description="本品牌及下属渠道的新请求统一使用此价格。" onConfirm={() => savePrice(item)}>保存价格</ConfirmButton><Button size="sm" variant="outline" onClick={() => setEditingPrice("")}>取消</Button></div>
            </div> : null}
          </li>
          ))}
        </ul>
      </ListResourceView>
    </Card>
  );
}
