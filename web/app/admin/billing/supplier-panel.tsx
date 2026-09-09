"use client";

import { useState } from "react";
import { ConfirmButton } from "@/components/confirm-button";
import { LedgerTable } from "@/components/console/ledger-table";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { IfCan } from "@/components/rbac/if-can";

type Supplier = {
  id?: string;
  channel_org_id?: string;
  amount_minor?: number;
  source_type?: string;
  vendor_name?: string;
  memo?: string;
  reversal_of?: string;
  created_at?: string;
};

function micro(n: unknown) {
  const v = Number(n);
  if (!Number.isFinite(v)) return "—";
  return `$${(v / 1_000_000).toFixed(2)}`;
}

function usdToMinor(raw: string) {
  const n = Number(raw);
  if (!Number.isFinite(n) || n <= 0) return 0;
  return Math.round(n * 1_000_000);
}

const selectClass =
  "h-10 min-h-10 w-full max-w-xs rounded-control border border-hairline bg-canvas-raised px-3 text-sm text-ink";

export function AdminSupplierPanel({ channelID }: { channelID?: string }) {
  const [items, setItems] = useState<Supplier[]>([]);
  const [usd, setUsd] = useState("10");
  const [sourceType, setSourceType] = useState("provider_invoice");
  const [vendor, setVendor] = useState("");
  const [memo, setMemo] = useState("");
  const [message, setMessage] = useState(
    channelID
      ? "这里只看该渠道账本。记一笔会记在当前登录人自己的账上（平台记官方渠道）。"
      : "线下已付给模型商，线上只记账。金额按正数填写，入库为负。记在官方渠道。",
  );

  const listPath = channelID
    ? `/admin/supplier-entries?channel_id=${encodeURIComponent(channelID)}`
    : "/admin/supplier-entries";

  async function load() {
    const res = await fetch(`${apiBase}${listPath}`, { credentials: "include" });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "读取供应商支出失败");
      return;
    }
    const next = Array.isArray(body.items) ? (body.items as Supplier[]) : [];
    setItems(next);
    setMessage(`供应商支出 ${next.length} 条`);
  }

  async function record() {
    const amount = usdToMinor(usd);
    if (!amount) {
      setMessage("请填写正数金额（USD）");
      return;
    }
    const res = await fetch(`${apiBase}/admin/supplier-entries`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({
        amount_minor: amount,
        source_type: sourceType,
        idempotency_key: `spe-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`,
        vendor_name: vendor || undefined,
        memo: memo || undefined,
      }),
    });
    const body = await res.json();
    setMessage(res.ok ? `已记账 ${body.item?.id}` : body.error?.message || "记账失败");
    if (res.ok) await load();
  }

  async function reverse(id: string) {
    const res = await fetch(`${apiBase}/admin/supplier-entries/${encodeURIComponent(id)}/reverse`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({ reason: "void" }),
    });
    const body = await res.json();
    setMessage(res.ok ? `已冲正 ${body.item?.id}` : body.error?.message || "冲正失败");
    if (res.ok) await load();
  }

  return (
    <section className="rounded-card border border-hairline bg-canvas-raised p-6">
      <h2 className="mb-4 text-lg font-semibold tracking-tight">供应商支出</h2>
      <p className="mb-3 text-sm text-ink-secondary">
        {channelID
          ? "列表按渠道过滤。新建仍记在平台官方账本，不会记到被查看的渠道。"
          : "A 记付给模型商。必填金额、来源类型；幂等键自动生成。"}
      </p>
      {!channelID ? (
        <IfCan action="billing.refund">
          <div className="mb-3 grid max-w-xl gap-2">
            <Input value={usd} onChange={(e) => setUsd(e.target.value)} aria-label="金额 USD" placeholder="10" />
            <select className={selectClass} aria-label="来源类型" value={sourceType} onChange={(e) => setSourceType(e.target.value)}>
              <option value="provider_invoice">付给模型商</option>
              <option value="platform_recharge">其他上游付款</option>
              <option value="other">其他</option>
            </select>
            <Input value={vendor} onChange={(e) => setVendor(e.target.value)} aria-label="对方名称" placeholder="厂商名" />
            <Input value={memo} onChange={(e) => setMemo(e.target.value)} aria-label="备注" placeholder="线下付款备忘" />
            <ConfirmButton size="sm" title="确认记供应商支出" description="线下已付，线上只记账。金额入库为负。" onConfirm={record}>
              记一笔
            </ConfirmButton>
          </div>
        </IfCan>
      ) : null}
      <Button size="sm" variant="outline" onClick={() => void load()}>
        刷新流水
      </Button>
      <LedgerTable
        columns={["时间", "金额", "来源", "渠道", "操作"]}
        emptyTitle="暂无供应商支出"
        emptyDetail="点刷新后可看到线下付款记账。"
        rows={items.map((item) => ({
          key: item.id || "spe",
          cells: [
            item.created_at ? String(item.created_at).slice(0, 19) : "—",
            <span key="a" className="font-mono tabular-nums">
              {micro(item.amount_minor)}
            </span>,
            item.source_type || "—",
            item.channel_org_id || "—",
            item.reversal_of ? (
              "—"
            ) : (
              <IfCan key="r" action="billing.refund">
                <ConfirmButton
                  size="sm"
                  variant="outline"
                  title="确认冲正"
                  description="成对补记，不删原流水。"
                  onConfirm={() => reverse(String(item.id))}
                >
                  冲正
                </ConfirmButton>
              </IfCan>
            ),
          ],
        }))}
      />
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </section>
  );
}
