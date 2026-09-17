"use client";

import { useState } from "react";
import { ConfirmButton } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { formatUsdMinor, parseUsdToMinor } from "@/lib/money";
import { channelUsesQuota } from "@/lib/tenants";

export function ChannelQuotaPanel({ channelID, channelType }: { channelID: string; channelType: string }) {
  const [amount, setAmount] = useState("1");
  const [ratioBPS, setRatioBPS] = useState("10000");
  const [message, setMessage] = useState("渠道额度按美元填写。发放和扣减都要二次确认。");

  async function loadQuota() {
    const res = await fetch(`${apiBase}/admin/channel-quotas/${channelID}`, { credentials: "include" });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "读取额度失败");
      return;
    }
    const bps = body.quota?.issue_ratio_bps ?? 10000;
    setRatioBPS(String(bps));
    setMessage(`可用额度 ${formatUsdMinor(body.quota?.available_minor)}，换算比 ${bps} BPS`);
  }

  async function saveRatio(): Promise<boolean> {
    try {
    const res = await fetch(`${apiBase}/admin/channel-quotas/${channelID}/issue-rule`, {
      method: "PATCH",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({ issue_ratio_bps: Number(ratioBPS) }),
    });
    const body = await res.json();
    setMessage(res.ok ? `已保存换算比 ${body.rule?.issue_ratio_bps} BPS` : body.error?.message || "保存换算比失败");
    const __ok = res.ok;
    return __ok;
    } catch {
      setMessage(confirmNetworkUnavailable);
      return false;
    }
}

  async function grant(): Promise<boolean> {
    try {
    const minor = parseUsdToMinor(amount);
    if (minor == null) {
      setMessage("请填写有效的美元金额");
      return false;
    }
    const res = await fetch(`${apiBase}/admin/channel-quotas/grant`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({ channel_org_id: channelID, amount_minor: minor }),
    });
    const body = await res.json();
    setMessage(res.ok ? `已调整 ${channelID}，可用 ${formatUsdMinor(body.quota?.available_minor)}` : body.error?.message || "调整失败");
    const __ok = res.ok;
    return __ok;
    } catch {
      setMessage(confirmNetworkUnavailable);
      return false;
    }
}

  return (
    <section className="rounded-stamp border border-hairline bg-canvas-raised p-6">
      <h2 className="mb-4 text-lg font-semibold tracking-tight">渠道额度</h2>
      {channelUsesQuota(channelType) ? (
        <>
          <p className="mb-3 text-sm text-ink-secondary">B/C 渠道可用额度在用户充值时按平台换算比发放，默认 1:1。正数授予，负数扣减。额度不足时不能再给新用户发放，预授权也会失败。</p>
          <div className="mb-3 flex flex-wrap gap-2">
            <Input className="w-40" value={amount} onChange={(e) => setAmount(e.target.value)} aria-label="额度 USD" placeholder="1.00" />
            <Button size="sm" variant="outline" onClick={loadQuota}>
              读取额度
            </Button>
            <ConfirmButton size="sm" title="确认调整额度" description="发放和扣减都会写审计，并从渠道 available 记账。" onConfirm={grant}>
              调整额度
            </ConfirmButton>
          </div>
          <h3 className="mb-2 mt-4 text-lg font-medium">换算比</h3>
          <p className="mb-3 text-sm text-ink-secondary">10000 BPS = 1:1。只有平台/财务能改，B/C 代理商不能改。合法范围 1000–100000（0.1x–10x）。</p>
          <div className="mb-3 flex flex-wrap gap-2">
            <Input className="w-40" value={ratioBPS} onChange={(e) => setRatioBPS(e.target.value)} aria-label="换算比 BPS" placeholder="10000" />
            <ConfirmButton size="sm" title="确认保存换算比" description="之后该渠道的充值会按新比例发放额度。" onConfirm={saveRatio}>
              保存换算比
            </ConfirmButton>
          </div>
        </>
      ) : (
        <p className="mb-3 text-sm text-ink-secondary">A 官方租户不发放渠道额度，终端用户直接充值。B/C 租户详情才需要调整额度。</p>
      )}
      <p className="text-sm text-ink-secondary">{message}</p>
    </section>
  );
}
