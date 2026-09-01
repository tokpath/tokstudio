"use client";

import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import { ChannelPaymentsNav } from "../payments-nav";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { apiBase } from "@/lib/api";

type Settings = {
  quick_amounts?: number[];
  min_pay_major?: number;
  max_pay_major?: number;
  timeout_minutes?: number;
  help_text?: string;
  help_image_url?: string;
  product_prefix?: string;
  product_suffix?: string;
  max_pending_orders?: number;
  cancel_rate_limit?: number;
  fee_bps?: number;
  issue_ratio_bps?: number;
  fen_per_usd?: number;
};

export default function ChannelPaymentRulesPage() {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: ["/channel/payments/settings"],
    queryFn: async () => {
      const res = await fetch(`${apiBase}/channel/payments/settings`, { credentials: "include" });
      const body = await res.json();
      return (body.item || {}) as Settings;
    },
  });
  const item = query.data;
  const [chips, setChips] = useState("");
  const [help, setHelp] = useState("");
  const [fee, setFee] = useState("");
  const [timeout, setTimeoutMin] = useState("");
  const [message, setMessage] = useState("");
  const [advanced, setAdvanced] = useState(false);

  const loaded = item
    ? {
        chips: chips || (item.quick_amounts || []).join(","),
        help: help || item.help_text || "",
        fee: fee || String(item.fee_bps ?? 0),
        timeout: timeout || String(item.timeout_minutes ?? 15),
      }
    : { chips: "", help: "", fee: "0", timeout: "15" };

  const previewMajor = 100;
  const feeBps = Number(loaded.fee) || 0;
  const fenPerUsd = item?.fen_per_usd || 715;
  const bps = item?.issue_ratio_bps || 10000;
  const payFen = previewMajor * 100;
  const feeFen = Math.floor((payFen * feeBps) / 10000);
  const wallet = Math.floor(((payFen - feeFen) * 1_000_000) / fenPerUsd);
  const credit = Math.floor((wallet * bps) / 10000);

  async function save() {
    const amounts = loaded.chips
      .split(",")
      .map((n) => Number(n.trim()))
      .filter((n) => n > 0);
    const res = await fetch(`${apiBase}/channel/payments/settings`, {
      method: "PATCH",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        quick_amounts: amounts,
        help_text: loaded.help,
        fee_bps: Number(loaded.fee),
        timeout_minutes: Number(loaded.timeout),
      }),
    });
    const body = await res.json();
    setMessage(res.ok ? "规则已保存。换算比仍由平台设定。" : body.error?.message || "保存失败");
    if (res.ok) await queryClient.invalidateQueries({ queryKey: ["/channel/payments/settings"] });
  }

  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelPaymentRules" />
      <ChannelPaymentsNav />
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <p className="th-eyebrow mb-3 text-ink-mute">LEDGER</p>
        <ul className="divide-y divide-hairline text-sm">
          <li className="flex justify-between py-2">
            <span>用户付</span>
            <span className="font-mono tabular-nums">¥{previewMajor.toFixed(2)}</span>
          </li>
          <li className="flex justify-between py-2">
            <span>手续费</span>
            <span className="font-mono tabular-nums">¥{(feeFen / 100).toFixed(2)}</span>
          </li>
          <li className="flex justify-between py-2">
            <span>钱包入账</span>
            <span className="font-mono tabular-nums">${(wallet / 1_000_000).toFixed(2)}</span>
          </li>
          <li className="flex justify-between py-2">
            <span>发放额度</span>
            <span className="font-mono tabular-nums">${(credit / 1_000_000).toFixed(2)}</span>
          </li>
        </ul>
        <p className="mt-3 text-sm text-ink-secondary">
          1 USD = {(fenPerUsd / 100).toFixed(2)} CNY；换算比 {bps} BPS。渠道不能改这个比例。
        </p>
      </section>
      <section className="rounded-card border border-hairline bg-canvas-raised p-6">
        <div className="grid max-w-xl gap-3">
          <Label htmlFor="chips">快捷金额（逗号分隔，3–8 个）</Label>
          <Input id="chips" value={loaded.chips} onChange={(e) => setChips(e.target.value)} placeholder="100,300,500,1000" />
          <Label htmlFor="timeout">订单超时（分钟）</Label>
          <Input id="timeout" value={loaded.timeout} onChange={(e) => setTimeoutMin(e.target.value)} />
          <Label htmlFor="help">帮助文案</Label>
          <Input id="help" value={loaded.help} onChange={(e) => setHelp(e.target.value)} />
          <button type="button" className="text-left text-sm text-brand-emphasis" onClick={() => setAdvanced((v) => !v)}>
            {advanced ? "收起高级" : "高级：商品名前缀、待支付上限、手续费"}
          </button>
          {advanced ? (
            <>
              <Label htmlFor="fee">手续费 BPS</Label>
              <Input id="fee" value={loaded.fee} onChange={(e) => setFee(e.target.value)} />
            </>
          ) : null}
          <Button className="w-fit" onClick={() => void save()}>
            保存规则
          </Button>
          {message ? <p className="text-sm text-ink-secondary">{message}</p> : null}
        </div>
      </section>
    </div>
  );
}
