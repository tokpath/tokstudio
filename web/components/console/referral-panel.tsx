"use client";

import Link from "next/link";
import { useViewer } from "@/components/rbac/viewer-context";
import { useBrand } from "@/components/brand-context";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import { RecoveryHistory } from "@/app/console/wallet/recovery-history";
import { useState } from "react";
import { useSearchParams } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { ListResourceView } from "@/components/console/list-resource-view";
import { LedgerTable } from "@/components/console/ledger-table";
import { PartnerBoard } from "@/app/partner/partner-board";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { formatUsdMinor } from "@/lib/money";

type Reward = { request_id?: string; reversal_of?: string; id: string; kind: string; status: string; amount_minor: number; available_at?: string };
type Settlement = { id: string; status: string; amount_minor: number; payout_reference?: string; reversed_minor: number; recovery_tracked: boolean; recovered_minor: number; recovery_pending_minor: number };
export type Referral = {
  codes: string[]; code_links: { code: string; share_url: string }[]; can_create: boolean; invited_count: number; can_commission: boolean; professional_customers: boolean;
  rules: { spend_minor: number; topup_minor: number; gift_minor: number };
  progress: { spend_minor: number; largest_topup_minor: number; gift_granted_minor: number; gift_remaining_minor: number };
  summary: { earned_minor: number; frozen_minor: number; available_minor: number; held_minor: number; settled_minor: number; paid_minor: number; reversed_minor: number };
  rewards: Reward[]; settlements: Settlement[];
  pagination: { page: number; page_size: number; rewards_total: number; settlements_total: number };
};

export function ReferralPanel() {
  const t = useTranslations("referral");
  const a = useTranslations("referralAccount");
  const partner = useTranslations("partnerBoard");
  const locale = useLocale();
  const search = useSearchParams();
  const viewer = useViewer();
  const brand = useBrand();
  const requestedTab = ["commissions", "settlements", "users"].includes(search.get("tab") || "") ? search.get("tab")! : "overview";
  const page = Math.max(1, Math.min(100000, Number(search.get("page")) || 1));
  const [creating, setCreating] = useState(false);
  const [notice, setNotice] = useState("");
  const [mutationError, setMutationError] = useState("");
  const resource = useListResource<Referral>({
    queryKey: `${viewer.userId || "anonymous"}|${brand?.id || ""}|${page}`,
    enabled: !viewer.loading,
    load: async () => {
      const res = await fetch(`${apiBase}/v1/me/referral?page=${page}`, { credentials: "include" });
      const body = await res.json();
      return { ok: res.ok, status: res.status, items: body.item ? [body.item] : [], message: body.error?.message, code: body.error?.code };
    },
  });
  const item = resource.snapshot.items[0];
  const showIncome=!!item && (item.can_commission || Object.values(item.summary).some(value=>value!==0) || item.pagination.rewards_total>0 || item.pagination.settlements_total>0);
  const tab=!showIncome && ["commissions","settlements"].includes(requestedTab) ? "overview" : requestedTab;
  function shareURL(code: string) {
    return item?.code_links.find(link => link.code === code)?.share_url || "";
  }
  async function copy(value: string) {
    setNotice("");
    try { await navigator.clipboard.writeText(value); setNotice(t("copied")); }
    catch { setNotice(t("copyFailed")); }
  }
  async function create() {
    setCreating(true); setMutationError("");
    try {
      const res = await fetch(`${apiBase}/v1/me/referral`, { method: "POST", credentials: "include" });
      if (!res.ok) throw new Error();
      await resource.reload();
    } catch { setMutationError(t("createFailed")); }
    finally { setCreating(false); }
  }
  const labels: Record<string, string> = Object.fromEntries(["direct", "indirect", "frozen", "available", "held", "settled", "paid", "reversed"].map(key => [key, t(key)]));
  const href = (nextTab: string, nextPage = 1) => `/app/referral?tab=${nextTab}${nextPage > 1 ? `&page=${nextPage}` : ""}`;
  const total = item ? tab === "settlements" ? item.pagination.settlements_total : item.pagination.rewards_total : 0;
  return <section className="flex flex-col gap-4" aria-label={t("title")}>
    <I18nConsoleHeader id="referral" actions={<Button variant="outline" disabled={resource.refreshing} onClick={() => void resource.reload()}>{t("refresh")}</Button>}/>
    <ListResourceView snapshot={resource.snapshot} emptyTitle={t("unavailable")} emptyDetail={t("retryDetail")} onRetry={() => void resource.reload()}>
      {item && <div className="flex flex-col gap-5">
        <nav aria-label={a("tabs")} className="flex flex-wrap gap-2">
          {["overview", ...(showIncome ? ["commissions", "settlements"] : []), ...(item.professional_customers ? ["users"] : [])].map(key => <Button key={key} asChild variant={tab === key ? "default" : "outline"}><Link href={href(key)} aria-current={tab === key ? "page" : undefined}>{a(key)}</Link></Button>)}
        </nav>
        {tab === "overview" && <>
          <Card><CardTitle>{t("share")}</CardTitle>

            {item.codes.map(code => <div key={code} className="my-4 space-y-2">
              <label className="block text-sm font-medium" htmlFor={`ref-${code}`}>{t("code")}: <span className="break-all font-mono">{code}</span></label>
              <input id={`ref-${code}`} aria-label={t("link")} readOnly value={shareURL(code)} onFocus={e => e.currentTarget.select()} className="w-full min-w-0 rounded-control border border-hairline bg-canvas px-3 py-2 text-sm" />
              <div className="flex flex-wrap gap-2"><Button disabled={!shareURL(code)} onClick={() => void copy(shareURL(code))}>{t("copyLink")}</Button><Button variant="outline" onClick={() => void copy(code)}>{t("copyCode")}</Button></div>
            </div>)}
            {!item.codes.length && (item.can_create ? <Button disabled={creating} onClick={() => void create()}>{creating ? t("creating") : t("create")}</Button> : <p>{t("unavailable")}</p>)}
            {notice && <p role="status" className="mt-2 text-sm">{notice}</p>}{mutationError && <p role="alert" className="mt-2 text-sm text-danger">{mutationError}</p>}
            <p className="mt-4 font-medium">{t("invited", { count: item.invited_count })}</p>
          </Card>
          <div className="grid gap-5 md:grid-cols-2">
            <Card><CardTitle>{a("giftTitle")}</CardTitle><dl className="mt-3 space-y-2 text-sm"><div className="flex justify-between gap-3"><dt>{a("giftGranted")}</dt><dd>{formatUsdMinor(item.progress.gift_granted_minor)}</dd></div><div className="flex justify-between gap-3"><dt>{a("giftRemaining")}</dt><dd>{formatUsdMinor(item.progress.gift_remaining_minor)}</dd></div></dl><p className="mt-3 text-sm text-ink-secondary">{a("giftDetail")}</p>{!item.can_commission && item.rules.gift_minor > 0 && <p className="mt-2 text-sm">{a("inviteeGift", { amount: formatUsdMinor(item.rules.gift_minor) })}</p>}</Card>
            <Card><CardTitle>{t("rules")}</CardTitle><p className="mt-3 font-medium">{item.can_commission ? t("qualified") : t("notQualified")}</p><p className="mt-2 text-sm text-ink-secondary">{a(item.can_commission ? "qualifiedDetail" : "qualifyDetail")}</p>
              <div className="mt-4 space-y-4">
                {item.rules.spend_minor > 0 && <Progress label={a("spendProgress")} value={item.progress.spend_minor} target={item.rules.spend_minor} />}
                {item.rules.topup_minor > 0 && <Progress label={a("topupProgress")} value={item.progress.largest_topup_minor} target={item.rules.topup_minor} />}
                {!item.can_commission && item.rules.spend_minor === 0 && item.rules.topup_minor === 0 && <p className="text-sm">{t("qualificationDisabled")}</p>}
              </div>
            </Card>
          </div>
          {showIncome && <Card><CardTitle>{a("incomeTitle")}</CardTitle><dl className="my-4 grid gap-4 sm:grid-cols-3">{["earned", "frozen", "available", "held", "settled", "paid"].map(key => <div key={key}><dt className="text-sm text-ink-secondary">{a(`income_${key}`)}</dt><dd className="mt-1 font-mono text-xl">{formatUsdMinor(item.summary[`${key}_minor` as keyof Referral["summary"]])}</dd></div>)}</dl><p className="mb-3 text-sm text-ink-secondary">{a("incomeDetail")}</p><Link href={href("commissions")} className="text-sm text-brand-emphasis underline">{a("viewIncome")}</Link></Card>}
        </>}
        {tab === "commissions" && <Card><CardTitle>{a("commissions")}</CardTitle><p className="my-3 text-sm text-ink-secondary">{a("incomeDetail")} {t("reversalHelp")}</p><LedgerTable columns={[t("kind"), t("status"), t("amount"), t("availableAt"), t("request"), t("entry")]} emptyTitle={t("emptyRewards")} emptyDetail={t("emptyRewardsDetail")} rows={item.rewards.map(row => ({ key: row.id, cells: [row.reversal_of ? t("reversal") : labels[row.kind] || t("other"), labels[row.status] || t("other"), formatUsdMinor(row.amount_minor), row.status === "frozen" && row.available_at ? new Date(row.available_at).toLocaleDateString(locale) : "—", <span className="block max-w-56 whitespace-normal break-all" key="request">{row.request_id || "—"}</span>, <div className="max-w-xs whitespace-normal break-all" key="entry">{row.id}{row.reversal_of && <p className="mt-1 text-ink-secondary">{t("original")}: {row.reversal_of}</p>}</div>] }))} /></Card>}
        {tab === "settlements" && <Card><CardTitle>{a("settlements")}</CardTitle><p className="my-3 text-sm text-ink-secondary">{a("settlementDetail")}</p><LedgerTable columns={[partner("colSettle"), partner("colStatus"), t("amount")]} emptyTitle={partner("emptySettle")} emptyDetail={partner("emptySettleDetail")} rows={item.settlements.map(row => ({key: row.id, cells: [<div key="id" className="max-w-xs whitespace-normal break-all">{row.id}{row.payout_reference && <p className="mt-1 text-ink-secondary">{partner("receipt")}: {row.payout_reference}</p>}</div>, <div key="status">{["settled", "paid", "cancelled"].includes(row.status) ? partner(`settlement_${row.status}`) : labels[row.status] || t("other")}{row.status === "paid" && row.reversed_minor > 0 && <p className="mt-2 text-danger">{row.recovery_tracked ? row.recovery_pending_minor > 0 ? a("recovery", { received: formatUsdMinor(row.recovered_minor), pending: formatUsdMinor(row.recovery_pending_minor) }) : partner("recoveryClosed") : partner("paidReversal", {amount: formatUsdMinor(row.reversed_minor)})}</p>}</div>, formatUsdMinor(row.amount_minor)]}))} /></Card>}
        {tab === "settlements" && <RecoveryHistory/>}
        {(tab === "commissions" || tab === "settlements") && <div className="flex items-center justify-between gap-3"><Button asChild variant="outline" disabled={page <= 1}><Link href={href(tab, Math.max(1, page-1))} aria-disabled={page <= 1}>{a("previous")}</Link></Button><p className="text-sm text-ink-secondary">{a("page", { page, total })}</p><Button asChild variant="outline" disabled={page*item.pagination.page_size >= total}><Link href={href(tab, page+1)} aria-disabled={page*item.pagination.page_size >= total}>{a("next")}</Link></Button></div>}
        {tab === "users" && (item.professional_customers ? <PartnerBoard section="users" /> : <Card><p>{a("noCustomerScope")}</p><Link className="mt-3 inline-block text-brand-emphasis underline" href={href("overview")}>{a("overview")}</Link></Card>)}
      </div>}
    </ListResourceView>
  </section>;
}
function Progress({label, value, target}: {label: string; value: number; target: number}) {
  return <div><div className="mb-2 flex justify-between gap-3 text-sm"><span>{label}</span><span>{formatUsdMinor(value)} / {formatUsdMinor(target)}</span></div><progress aria-label={label} value={Math.min(value,target)} max={target} className="h-2 w-full accent-brand" /></div>;
}
