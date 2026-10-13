"use client";

import { useTranslations } from "next-intl";
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

export function PolicyEditor({ prefix = "/admin", canEdit = false, scope = "", ownerLabel = "" }: { scope?: string; ownerLabel?: string; prefix?: "/admin" | "/channel"; canEdit?: boolean }) {
  const tUI = useTranslations();
  const viewer = useViewer();
  const [policy, setPolicy] = useState(emptyPolicy);
  const [rule, setRule] = useState(emptyRule);
  const [version, setVersion] = useState("");
  const [message, setMessage] = useState("");
  const policyQuery = useQuery({ queryKey: [viewer.userId, scope, prefix, "commission-policy"], queryFn: () => apiClient<{ policy?: Policy; error?: { message?: string } }>("GET", `${prefix}/commission-policy`), refetchOnWindowFocus: false });
  const ruleQuery = useQuery({ queryKey: [viewer.userId, scope, prefix, "eligibility-rules"], queryFn: () => apiClient<{ rule?: Rule; error?: { message?: string } }>("GET", `${prefix}/eligibility-rules`), refetchOnWindowFocus: false });
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
      setMessage(tUI("consoleCopy.enterValidPercentagesFreezeDaysAndAmountsDirect")); return false;
    }
    try {
      const response = await fetch(`${apiBase}${prefix}/commission-policy`, { method: "PATCH", credentials: "include", headers: confirmHeaders, body: JSON.stringify({ direct_bps: direct, indirect_bps: indirect, total_bps: total, freeze_days: freeze, min_settle_minor: minimum, expected_version: version }) });
      const body = await response.json();
      if (!response.ok) { setMessage(body.error?.message || tUI("consoleCopy.saveFailedYourCurrentInputHasBeenRetained")); return false; }
      setMessage(tUI("consoleCopy.policySavedAppliesOnlyToFutureUsage", { value0: String(body.policy?.version) }));
      await policyQuery.refetch(); return true;
    } catch { setMessage(tUI("consoleCopy.theResultIsUnconfirmedReloadAndCheckThe")); return false; }
  }
  async function saveRule() {
    if (!canEdit || !ruleReady) return false;
    const spend = parseUsdToMinor(rule.spend), topup = parseUsdToMinor(rule.topup), gift = parseUsdToMinor(rule.gift);
    if ([spend, topup, gift].some(v => v === null)) { setMessage(tUI("consoleCopy.enterAValidUsdAmountSet0To")); return false; }
    try {
      const response = await fetch(`${apiBase}${prefix}/eligibility-rules`, { method: "PATCH", credentials: "include", headers: confirmHeaders, body: JSON.stringify({ spend_minor: spend, topup_minor: topup, gift_minor: gift }) });
      const body = await response.json();
      if (!response.ok) { setMessage(body.error?.message || tUI("user.saveFail")); return false; }
      setMessage(tUI("consoleCopy.qualificationRulesSavedAppliesOnlyToFutureQualification")); await ruleQuery.refetch(); return true;
    } catch { setMessage(tUI("consoleCopy.theResultIsUnconfirmedReloadTheRulesAnd")); return false; }
  }
  return <div className="space-y-6">
    <section className="rounded-card border border-hairline bg-canvas-raised p-6" aria-label={tUI("consoleCopy.commissionPolicy")}>
      <h2 className="text-lg font-semibold">{tUI("consoleCopy.commissionPolicy")}</h2>
      <p className="my-3 text-sm text-ink-secondary">{tUI("consoleCopy.currentVersion")}{policyReady ? version : tUI("consoleCopy.notYetLoaded")}{tUI("consoleCopy.historicalUsageRetainsTheOriginalPolicy")}</p>
      {!policyReady ? <p role="status">{policyQuery.isFetching ? tUI("consoleCopy.loadingCurrentPolicy") : policyQuery.data?.error?.message || tUI("consoleCopy.couldNotLoadThePolicyPleaseRetry")}</p> : null}
      <fieldset disabled={!canEdit || !policyReady} className="my-4 grid max-w-3xl gap-3 sm:grid-cols-3">
        {([{ key: "direct", label: tUI("consoleCopy.directCommission") }, { key: "indirect", label: tUI("consoleCopy.indirectCommission") }, { key: "total", label: tUI("consoleCopy.totalCommission") }, { key: "freeze", label: tUI("channelUi.freezeDays") }, { key: "minimum", label: tUI("consoleCopy.minimumSettlementUsd") }] as const).map(field => <label key={field.key} className="grid gap-1 text-sm"><span>{field.label}</span><Input aria-label={field.label} value={policy[field.key]} onChange={e => setPolicy(p => ({ ...p, [field.key]: e.target.value }))} inputMode="decimal" /></label>)}
      </fieldset>
      <div className="flex flex-wrap gap-2"><Button size="sm" variant="outline" onClick={() => void policyQuery.refetch()}>{tUI("consoleCopy.reloadPolicy")}</Button>{canEdit ? <ConfirmButton size="sm" disabled={!policyReady} title={tUI("consoleCopy.saveCommissionPolicy")} description={tUI("consoleCopy.brandVersionDirectIndirectTotalFreezeDaysMinimum", { value0: String(ownerLabel || tUI("consoleCopy.currentAuthorizedScope")), value1: String(version), value2: String(bpsToPercent(policyQuery.data?.policy?.direct_bps || 0)), value3: String(policy.direct), value4: String(bpsToPercent(policyQuery.data?.policy?.indirect_bps || 0)), value5: String(policy.indirect), value6: String(bpsToPercent(policyQuery.data?.policy?.total_bps || 0)), value7: String(policy.total), value8: String(policyQuery.data?.policy?.freeze_days), value9: String(policy.freeze), value10: String(Number(policyQuery.data?.policy?.min_settle_minor || 0) / 1_000_000), value11: String(policy.minimum) })} onConfirm={savePolicy}>{tUI("consoleCopy.savePolicy")}</ConfirmButton> : null}</div>
    </section>
    <section className="rounded-card border border-hairline bg-canvas-raised p-6" aria-label={tUI("consoleCopy.referralCommissionEligibility")}>
      <h2 className="text-lg font-semibold">{tUI("consoleCopy.referralCommissionEligibility")}</h2>
      {!ruleReady ? <p role="status">{ruleQuery.isFetching ? tUI("consoleCopy.loadingQualificationRules") : ruleQuery.data?.error?.message || tUI("consoleCopy.couldNotLoadQualificationRulesPleaseRetry")}</p> : null}
      <fieldset disabled={!canEdit || !ruleReady} className="my-4 grid max-w-3xl gap-3 sm:grid-cols-3">
        {([{ key: "spend", label: tUI("consoleCopy.cumulativeUsageThresholdUsd") }, { key: "topup", label: tUI("consoleCopy.singleTopUpThresholdUsd") }, { key: "gift", label: tUI("consoleCopy.registrationGiftForAnIneligibleReferralUsdCredit") }] as const).map(field => <label key={field.key} className="grid gap-1 text-sm"><span>{field.label}</span><Input aria-label={field.label} value={rule[field.key]} onChange={e => setRule(r => ({ ...r, [field.key]: e.target.value }))} inputMode="decimal" /></label>)}
      </fieldset>
      <div className="flex flex-wrap gap-2"><Button size="sm" variant="outline" onClick={() => void ruleQuery.refetch()}>{tUI("consoleCopy.reloadQualificationRules")}</Button>{canEdit ? <ConfirmButton size="sm" disabled={!ruleReady} title={tUI("consoleCopy.saveQualificationRules")} description={tUI("consoleCopy.appliesOnlyToFutureQualificationAndRegistrationGifts")} onConfirm={saveRule}>{tUI("channelUi.saveElig")}</ConfirmButton> : null}</div>
    </section>
    {message ? <p role="status" className="text-sm text-ink-secondary">{message}</p> : null}
  </div>;
}
