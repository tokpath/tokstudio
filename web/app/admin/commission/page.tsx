"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ConfirmButton } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan } from "@/components/rbac/if-can";

type Policy = {
  id?: string;
  version?: string;
  direct_bps?: number;
  indirect_bps?: number;
  total_bps?: number;
  freeze_days?: number;
  min_settle_minor?: number;
};

type Eligibility = {
  spend_minor?: number;
  topup_minor?: number;
  gift_minor?: number;
};

type Commission = { id: string; kind: string; status: string; amount_minor: number; channel_org_id?: string };
type Settlement = { id: string; status: string; amount_minor: number; channel_org_id?: string };

export default function AdminCommissionPage() {
  const [direct, setDirect] = useState("1500");
  const [indirect, setIndirect] = useState("500");
  const [total, setTotal] = useState("2500");
  const [freeze, setFreeze] = useState("7");
  const [minSettle, setMinSettle] = useState("1");
  const [spend, setSpend] = useState("10");
  const [topup, setTopup] = useState("10");
  const [gift, setGift] = useState("1");
  const [eligMessage, setEligMessage] = useState("累计消费 / 单笔充值达线；无资格分享发注册赠送积分。金额是 USD。");
  const [usageEventID, setUsageEventID] = useState("");
  const [recalcUsageID, setRecalcUsageID] = useState("");
  const [recalcMessage, setRecalcMessage] = useState("按 usage 上的价格快照冲正旧流水，再挂新冻结额。需要二次确认。");
  const [settlementID, setSettlementID] = useState("");
  const [payoutRef, setPayoutRef] = useState("manual-wire");
  const [message, setMessage] = useState("BPS 是万分比。直接+间接不能超过总佣金。保存、解冻、结算和打款都要二次确认。");

  const policyQuery = useQuery({
    queryKey: ["commission-policy"],
    queryFn: () => apiClient<{ policy?: Policy; error?: { message?: string } }>("GET", "/admin/commission-policy"),
  });

  async function loadPolicy() {
    const res = await fetch(`${apiBase}/admin/commission-policy`, { credentials: "include" });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "未登录平台管理员");
      return;
    }
    const p = body.policy as Policy;
    setDirect(String(p.direct_bps ?? 1500));
    setIndirect(String(p.indirect_bps ?? 0));
    setTotal(String(p.total_bps ?? 2500));
    setFreeze(String(p.freeze_days ?? 7));
    setMinSettle(String((p.min_settle_minor ?? 1_000_000) / 1_000_000));
    setMessage(`已读取策略 ${p.version || p.id}`);
  }

  async function savePolicy(): Promise<boolean> {
    try {
    const res = await fetch(`${apiBase}/admin/commission-policy`, {
      method: "PATCH",
      credentials: "include",
      headers: confirmHeaders,
        body: JSON.stringify({
        direct_bps: Number(direct),
        indirect_bps: Number(indirect),
        total_bps: Number(total),
        freeze_days: Number(freeze),
        min_settle_minor: Math.round(Number(minSettle) * 1_000_000),
      }),
    });
    const body = await res.json();
    setMessage(res.ok ? `已保存 ${body.policy?.version}，直接佣金 ${body.policy?.direct_bps} bps` : body.error?.message || "保存失败");
    const __ok = res.ok;
    return __ok;
    } catch {
      setMessage(confirmNetworkUnavailable);
      return false;
    }
}

  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="commission" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">
          直接 / 间接 / 总佣金与冻结天数。改策略只影响之后的 usage，不改已经入账的明细。
        </p>
        <div className="mb-3 grid max-w-3xl grid-cols-2 gap-2 md:grid-cols-3">
          <Input value={direct} onChange={(e) => setDirect(e.target.value)} aria-label="直接佣金 BPS" placeholder="direct_bps" />
          <Input value={indirect} onChange={(e) => setIndirect(e.target.value)} aria-label="间接佣金 BPS" placeholder="indirect_bps" />
          <Input value={total} onChange={(e) => setTotal(e.target.value)} aria-label="总佣金 BPS" placeholder="total_bps" />
          <Input value={freeze} onChange={(e) => setFreeze(e.target.value)} aria-label="冻结天数" placeholder="freeze_days" />
          <Input value={minSettle} onChange={(e) => setMinSettle(e.target.value)} aria-label="最低结算额 USD" placeholder="1" />
        </div>
        <div className="flex flex-wrap gap-2">
          <Button size="sm" variant="outline" onClick={loadPolicy}>
            读取策略
          </Button>
          <IfCan action="commission.write">
          <ConfirmButton size="sm" title="确认保存策略" description="改策略只影响之后的 usage，不改已经入账的明细。" onConfirm={savePolicy}>
            保存策略
          </ConfirmButton>
          </IfCan>
        </div>
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
        {policyQuery.data?.error ? <p className="mt-2 text-sm text-ink-secondary">{policyQuery.data.error.message}</p> : null}
      </section>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <h2 className="mb-4 text-lg font-semibold tracking-tight">达线规则</h2>
        <p className="mb-3 text-sm text-ink-secondary">平台默认达线。C 可在渠道台覆盖。金额是 USD。</p>
        <div className="mb-3 grid max-w-3xl grid-cols-2 gap-2 md:grid-cols-3">
          <Input value={spend} onChange={(e) => setSpend(e.target.value)} aria-label="累计消费达线 USD" placeholder="10" />
          <Input value={topup} onChange={(e) => setTopup(e.target.value)} aria-label="单笔充值达线 USD" placeholder="10" />
          <Input value={gift} onChange={(e) => setGift(e.target.value)} aria-label="注册赠送 USD" placeholder="1" />
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            size="sm"
            variant="outline"
            onClick={async () => {
              const res = await fetch(`${apiBase}/admin/eligibility-rules`, { credentials: "include" });
              const body = await res.json();
              if (!res.ok) {
                setEligMessage(body.error?.message || "读取达线失败");
                return;
              }
              const r = body.rule as Eligibility;
              setSpend(String((r.spend_minor ?? 0) / 1_000_000));
              setTopup(String((r.topup_minor ?? 0) / 1_000_000));
              setGift(String((r.gift_minor ?? 0) / 1_000_000));
              setEligMessage("已读取平台达线");
            }}
          >
            读取达线
          </Button>
          <IfCan action="commission.write">
            <ConfirmButton
              size="sm"
              title="确认保存达线"
              description="只影响之后的达线和注册赠送。"
              onConfirm={async () => {
                    try {
                const res = await fetch(`${apiBase}/admin/eligibility-rules`, {
                  method: "PATCH",
                  credentials: "include",
                  headers: confirmHeaders,
                  body: JSON.stringify({
                    spend_minor: Math.round(Number(spend) * 1_000_000),
                    topup_minor: Math.round(Number(topup) * 1_000_000),
                    gift_minor: Math.round(Number(gift) * 1_000_000),
                  }),
                });
                const body = await res.json();
                setEligMessage(res.ok ? "已保存达线规则" : body.error?.message || "保存失败");
                    const __ok = res.ok;
                    return __ok;
                    } catch {
                      setEligMessage(confirmNetworkUnavailable);
                      return false;
                    }
}}
            >
              保存达线
            </ConfirmButton>
          </IfCan>
        </div>
        <p className="mt-3 text-sm text-ink-secondary">{eligMessage}</p>
      </section>
      <IfCan action="commission.write">
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="recalc" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">用当时价格快照重算，不改历史账单单价。缺确认会 409。</p>
        <div className="flex flex-wrap items-center gap-2">
          <Input
            className="w-72"
            value={recalcUsageID}
            onChange={(e) => setRecalcUsageID(e.target.value)}
            aria-label="重算用 usage 事件 ID"
            placeholder="重算用 usage_event_id"
          />
          <ConfirmButton
            size="sm"
            title="确认重算佣金"
            description="用当时价格快照重算，不改历史账单单价。"
            onConfirm={async () => {
                    try {
              const res = await fetch(`${apiBase}/admin/commissions/recalc`, {
                method: "POST",
                credentials: "include",
                headers: confirmHeaders,
                body: JSON.stringify({ usage_event_id: recalcUsageID }),
              });
              const body = await res.json();
              setRecalcMessage(
                res.ok
                  ? `已重算 ${body.item?.id || body.item?.usage_event_id} → ${body.item?.status} / ${body.item?.policy_version}`
                  : body.error?.message || "重算失败",
              );
                    return true;
                    } catch {
                      setRecalcMessage(confirmNetworkUnavailable);
                      return false;
                    }
}}
          >
            重算佣金
          </ConfirmButton>
          <p className="text-sm text-ink-secondary">{recalcMessage}</p>
        </div>
      </section>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="manualSettle" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">P0 只做人工解冻、生成月结单和打款。自动代付不在范围内。</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-64" value={usageEventID} onChange={(e) => setUsageEventID(e.target.value)} aria-label="usage 事件 ID" placeholder="usage_event_id" />
          <ConfirmButton
            size="sm"
            variant="outline"
            title="确认解冻佣金"
            description="按 usage 事件解冻已到期的冻结额。"
            onConfirm={async () => {
                    try {
              const res = await fetch(`${apiBase}/admin/commissions/unfreeze`, {
                method: "POST",
                credentials: "include",
                headers: confirmHeaders,
                body: JSON.stringify({ usage_event_id: usageEventID }),
              });
              const body = await res.json();
              setMessage(res.ok ? `已解冻 ${body.unfrozen} 条` : body.error?.message || "解冻失败");
                    const __ok = res.ok;
                    return __ok;
                    } catch {
                      setMessage(confirmNetworkUnavailable);
                      return false;
                    }
}}
          >
            解冻佣金
          </ConfirmButton>
          <ConfirmButton
            size="sm"
            title="确认生成结算单"
            description="P0 只做人工结算，自动代付不在范围内。"
            onConfirm={async () => {
                    try {
              const res = await fetch(`${apiBase}/admin/commissions/settle?ignore_minimum=1`, {
                method: "POST",
                credentials: "include",
                headers: confirmHeaders,
                body: "{}",
              });
              const body = await res.json();
              setMessage(res.ok ? `已生成 ${body.items?.length ?? 0} 张结算单` : body.error?.message || "结算失败");
                    const __ok = res.ok;
                    return __ok;
                    } catch {
                      setMessage(confirmNetworkUnavailable);
                      return false;
                    }
}}
          >
            生成结算单
          </ConfirmButton>
        </div>
        <div className="flex flex-wrap gap-2">
          <Input className="w-64" value={settlementID} onChange={(e) => setSettlementID(e.target.value)} aria-label="结算单 ID" placeholder="csl_..." />
          <Input className="w-40" value={payoutRef} onChange={(e) => setPayoutRef(e.target.value)} aria-label="打款凭证" placeholder="reference" />
          <ConfirmButton
            size="sm"
            title="确认人工打款"
            description="只记录人工打款凭证，不会自动代付。"
            onConfirm={async () => {
                    try {
              const res = await fetch(`${apiBase}/admin/settlements/${settlementID}/payout`, {
                method: "POST",
                credentials: "include",
                headers: confirmHeaders,
                body: JSON.stringify({ method: "manual", reference: payoutRef }),
              });
              const body = await res.json();
              setMessage(res.ok ? `已打款 ${body.item?.id} → ${body.item?.status}` : body.error?.message || "打款失败");
                    const __ok = res.ok;
                    return __ok;
                    } catch {
                      setMessage(confirmNetworkUnavailable);
                      return false;
                    }
}}
          >
            人工打款
          </ConfirmButton>
        </div>
      </section>
      </IfCan>
      <AdminListPanel<Commission>
        path="/admin/commissions"
        title="佣金明细"
        columns={[
          { accessorKey: "kind", header: "Kind" },
          { accessorKey: "status", header: "Status" },
          { accessorKey: "amount_minor", header: "Amount" },
          { accessorKey: "channel_org_id", header: "Channel" },
        ]}
      />
      <AdminListPanel<Settlement>
        path="/admin/settlements"
        title="结算单"
        columns={[
          { accessorKey: "id", header: "ID" },
          { accessorKey: "status", header: "Status" },
          { accessorKey: "amount_minor", header: "Amount" },
          { accessorKey: "channel_org_id", header: "Channel" },
        ]}
      />
    </AdminShell>
  );
}
