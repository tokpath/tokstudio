"use client";

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { apiClient } from "@/lib/client";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { bpsToPercent, percentToBps } from "@/lib/commission-percent";
import { parseUsdToMinor } from "@/lib/money";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useViewer } from "@/components/rbac/viewer-context";
import { ConfirmButton } from "@/components/confirm-button";

type Policy = { version: string; direct_bps: number; indirect_bps: number; total_bps: number; freeze_days: number; min_settle_minor: number };
type Rule = { spend_minor: number; topup_minor: number; gift_minor: number };
const emptyPolicy = { direct: "", indirect: "", total: "", freeze: "", minimum: "" };
const emptyRule = { spend: "", topup: "", gift: "" };

export function PolicyEditor({ prefix = "/admin", canEdit = false }: { prefix?: "/admin" | "/channel"; canEdit?: boolean }) {
  const viewer = useViewer();
  const [policy, setPolicy] = useState(emptyPolicy);
  const [rule, setRule] = useState(emptyRule);
  const [version, setVersion] = useState("");
  const [message, setMessage] = useState("");
  const policyQuery = useQuery({ queryKey: [viewer.userId, prefix, "commission-policy"], queryFn: () => apiClient<{ policy?: Policy; error?: { message?: string } }>("GET", `${prefix}/commission-policy`), refetchOnWindowFocus: false });
  const ruleQuery = useQuery({ queryKey: [viewer.userId, prefix, "eligibility-rules"], queryFn: () => apiClient<{ rule?: Rule; error?: { message?: string } }>("GET", `${prefix}/eligibility-rules`), refetchOnWindowFocus: false });
  useEffect(() => {
    const p = policyQuery.data?.policy;
    if (!p?.version) return;
    setVersion(p.version);
    setPolicy({ direct: bpsToPercent(p.direct_bps), indirect: bpsToPercent(p.indirect_bps), total: bpsToPercent(p.total_bps), freeze: String(p.freeze_days), minimum: String(p.min_settle_minor / 1_000_000) });
  }, [policyQuery.data]);
  useEffect(() => {
    const r = ruleQuery.data?.rule;
    if (r) setRule({ spend: String(r.spend_minor / 1_000_000), topup: String(r.topup_minor / 1_000_000), gift: String(r.gift_minor / 1_000_000) });
  }, [ruleQuery.data]);
  const policyReady = Boolean(version && policyQuery.data?.policy && !policyQuery.data?.error && !policyQuery.isError && !policyQuery.isFetching);
  const ruleReady = Boolean(ruleQuery.data?.rule && !ruleQuery.data?.error && !ruleQuery.isError && !ruleQuery.isFetching);
  async function savePolicy() {
    if (!canEdit || !policyReady) return false;
    const direct = percentToBps(policy.direct), indirect = percentToBps(policy.indirect), total = percentToBps(policy.total);
    const minimum = parseUsdToMinor(policy.minimum), freeze = Number(policy.freeze);
    if (direct === null || indirect === null || total === null || total <= 0 || direct + indirect > total || minimum === null || !Number.isInteger(freeze) || freeze < 0 || freeze > 90) {
      setMessage("请填写有效百分比、冻结天数及金额；直接与间接比例之和不能超过总佣金。"); return false;
    }
    try {
      const response = await fetch(`${apiBase}${prefix}/commission-policy`, { method: "PATCH", credentials: "include", headers: confirmHeaders, body: JSON.stringify({ direct_bps: direct, indirect_bps: indirect, total_bps: total, freeze_days: freeze, min_settle_minor: minimum, expected_version: version }) });
      const body = await response.json();
      if (!response.ok) { setMessage(body.error?.message || "保存失败，保留当前内容。"); return false; }
      setMessage(`已保存策略 ${body.policy?.version}；仅影响之后的新消费。`);
      await policyQuery.refetch(); return true;
    } catch { setMessage("结果尚未确认，请重新读取当前策略后核对；不要直接重复覆盖。"); return false; }
  }
  async function saveRule() {
    if (!canEdit || !ruleReady) return false;
    const spend = parseUsdToMinor(rule.spend), topup = parseUsdToMinor(rule.topup), gift = parseUsdToMinor(rule.gift);
    if ([spend, topup, gift].some(v => v === null)) { setMessage("请填写有效 USD 金额，0 表示关闭对应路径。"); return false; }
    try {
      const response = await fetch(`${apiBase}${prefix}/eligibility-rules`, { method: "PATCH", credentials: "include", headers: confirmHeaders, body: JSON.stringify({ spend_minor: spend, topup_minor: topup, gift_minor: gift }) });
      const body = await response.json();
      if (!response.ok) { setMessage(body.error?.message || "保存失败"); return false; }
      setMessage("已保存达线规则；仅影响之后的达线与注册赠送。"); await ruleQuery.refetch(); return true;
    } catch { setMessage("结果尚未确认，请重新读取规则后核对。"); return false; }
  }
  return <div className="space-y-6">
    <section className="rounded-card border border-hairline bg-canvas-raised p-6" aria-label="分佣策略">
      <h2 className="text-lg font-semibold">分佣策略</h2>
      <p className="my-3 text-sm text-ink-secondary">当前版本：{policyReady ? version : "尚未读取"}。历史消费保留原策略。</p>
      {!policyReady ? <p role="status">{policyQuery.isFetching ? "正在读取当前策略…" : policyQuery.data?.error?.message || "策略读取失败，请重试。"}</p> : null}
      <fieldset disabled={!canEdit || !policyReady} className="my-4 grid max-w-3xl gap-3 sm:grid-cols-3">
        {([{ key: "direct", label: "直接佣金（%）" }, { key: "indirect", label: "间接佣金（%）" }, { key: "total", label: "总佣金（%）" }, { key: "freeze", label: "冻结天数" }, { key: "minimum", label: "最低结算额（USD）" }] as const).map(field => <label key={field.key} className="grid gap-1 text-sm"><span>{field.label}</span><Input aria-label={field.label} value={policy[field.key]} onChange={e => setPolicy(p => ({ ...p, [field.key]: e.target.value }))} inputMode="decimal" /></label>)}
      </fieldset>
      <div className="flex flex-wrap gap-2"><Button size="sm" variant="outline" onClick={() => void policyQuery.refetch()}>重新读取策略</Button>{canEdit ? <ConfirmButton size="sm" disabled={!policyReady} title="保存分佣策略" description={`直接 ${policy.direct}%、间接 ${policy.indirect}%、总佣金 ${policy.total}%；以版本 ${version} 为基础，仅影响新消费。`} onConfirm={savePolicy}>保存策略</ConfirmButton> : null}</div>
    </section>
    <section className="rounded-card border border-hairline bg-canvas-raised p-6" aria-label="邀请分佣资格">
      <h2 className="text-lg font-semibold">邀请分佣资格</h2>
      {!ruleReady ? <p role="status">{ruleQuery.isFetching ? "正在读取达线规则…" : ruleQuery.data?.error?.message || "达线规则读取失败，请重试。"}</p> : null}
      <fieldset disabled={!canEdit || !ruleReady} className="my-4 grid max-w-3xl gap-3 sm:grid-cols-3">
        {([{ key: "spend", label: "累计消费达线（USD）" }, { key: "topup", label: "单笔充值达线（USD）" }, { key: "gift", label: "无资格邀请注册赠送（USD 积分）" }] as const).map(field => <label key={field.key} className="grid gap-1 text-sm"><span>{field.label}</span><Input aria-label={field.label} value={rule[field.key]} onChange={e => setRule(r => ({ ...r, [field.key]: e.target.value }))} inputMode="decimal" /></label>)}
      </fieldset>
      <div className="flex flex-wrap gap-2"><Button size="sm" variant="outline" onClick={() => void ruleQuery.refetch()}>重新读取达线</Button>{canEdit ? <ConfirmButton size="sm" disabled={!ruleReady} title="保存达线规则" description="只影响之后的达线和注册赠送。充值本身不计佣。" onConfirm={saveRule}>保存达线</ConfirmButton> : null}</div>
    </section>
    {message ? <p role="status" className="text-sm text-ink-secondary">{message}</p> : null}
  </div>;
}
