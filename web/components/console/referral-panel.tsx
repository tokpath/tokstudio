"use client";

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { ListResourceView } from "@/components/console/list-resource-view";
import { LedgerTable } from "@/components/console/ledger-table";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { formatUsdMinor } from "@/lib/money";

type Referral = {
  codes: string[];
  can_create: boolean;
  invited_count: number;
  can_commission: boolean;
  rules: { spend_minor: number; topup_minor: number; gift_minor: number };
  rewards: { id: string; kind: string; status: string; amount_minor: number; available_at?: string }[];
};

export function ReferralPanel() {
  const t = useTranslations("referral");
  const locale = useLocale();
  const [creating, setCreating] = useState(false);
  const [notice, setNotice] = useState("");
  const [mutationError, setMutationError] = useState("");
  const resource = useListResource<Referral>({
    load: async () => {
      const res = await fetch(`${apiBase}/v1/me/referral`, { credentials: "include" });
      const body = await res.json();
      return { ok: res.ok, status: res.status, items: body.item ? [body.item] : [], message: body.error?.message, code: body.error?.code };
    },
  });
  const item = resource.snapshot.items[0];
  function shareURL(code: string) {
    const url = new URL("/login", window.location.origin);
    url.searchParams.set("promotion_code", code);
    return url.toString();
  }
  async function copy(value: string) {
    setNotice("");
    try { await navigator.clipboard.writeText(value); setNotice(t("copied")); }
    catch { setNotice(t("copyFailed")); }
  }
  async function create() {
    setCreating(true);
    setMutationError("");
    try {
      const res = await fetch(`${apiBase}/v1/me/referral`, { method: "POST", credentials: "include" });
      if (!res.ok) throw new Error();
      await resource.reload();
    } catch { setMutationError(t("createFailed")); }
    finally { setCreating(false); }
  }
  const labels: Record<string, string> = {
    direct: t("direct"), indirect: t("indirect"), frozen: t("frozen"), available: t("available"),
    held: t("held"), settled: t("settled"), paid: t("paid"), reversed: t("reversed"),
  };
  return (
    <section className="flex flex-col gap-4" aria-label={t("title")}>
      <div className="flex items-center justify-between gap-3">
        <h2 className="text-lg font-semibold">{t("title")}</h2>
        <Button variant="outline" disabled={resource.refreshing} onClick={() => void resource.reload()}>{t("refresh")}</Button>
      </div>
      <ListResourceView snapshot={resource.snapshot} emptyTitle={t("unavailable")} emptyDetail={t("retryDetail")} onRetry={() => void resource.reload()}>
        {item && <div className="flex flex-col gap-4">
          <Card>
            <CardTitle>{t("share")}</CardTitle>
            <p className="mb-4 text-sm text-ink-secondary">{t("shareDetail")}</p>
            {item.codes.map(code => <div key={code} className="mb-4 space-y-2">
              <label className="block text-sm font-medium" htmlFor={`ref-${code}`}>{t("code")}: {code}</label>
              <input id={`ref-${code}`} aria-label={t("link")} readOnly value={shareURL(code)} onFocus={e => e.currentTarget.select()} className="w-full min-w-0 rounded-control border border-hairline bg-canvas px-3 py-2 text-sm" />
              <div className="flex flex-wrap gap-2">
                <Button onClick={() => void copy(shareURL(code))}>{t("copyLink")}</Button>
                <Button variant="outline" onClick={() => void copy(code)}>{t("copyCode")}</Button>
              </div>
            </div>)}
            {!item.codes.length && (item.can_create
              ? <Button disabled={creating} onClick={() => void create()}>{creating ? t("creating") : t("create")}</Button>
              : <p>{t("unavailable")}</p>)}
            {notice && <p role="status" className="mt-2 text-sm">{notice}</p>}
            {mutationError && <p role="alert" className="mt-2 text-sm text-danger">{mutationError}</p>}
            <p className="mt-4 text-sm">{t("invited", { count: item.invited_count })}</p>
          </Card>
          <Card>
            <CardTitle>{t("rules")}</CardTitle>
            <p className="mt-2 font-medium">{item.can_commission ? t("qualified") : t("notQualified")}</p>
            <p className="mt-2 text-sm text-ink-secondary">{item.can_commission ? t("commissionDetail") : item.rules.gift_minor > 0 ? t("giftDetail", { amount: formatUsdMinor(item.rules.gift_minor) }) : t("giftDisabled")}</p>
            {!item.can_commission && <div className="mt-3 space-y-1 text-sm">
              {(item.rules.spend_minor > 0 || item.rules.topup_minor > 0) ? <>
                <p>{t("qualify")}</p>
                {item.rules.spend_minor > 0 && <p>{t("spend", { amount: formatUsdMinor(item.rules.spend_minor) })}</p>}
                {item.rules.topup_minor > 0 && <p>{t("topup", { amount: formatUsdMinor(item.rules.topup_minor) })}</p>}
              </> : <p>{t("qualificationDisabled")}</p>}
            </div>}
          </Card>
          <Card>
            <CardTitle>{t("rewards")}</CardTitle>
            <p className="my-2 text-sm text-ink-secondary">{t("rewardsDetail")}</p>
            <LedgerTable columns={[t("kind"), t("status"), t("amount"), t("availableAt")]} emptyTitle={t("emptyRewards")} emptyDetail={t("emptyRewardsDetail")} rows={item.rewards.map(row => ({ key: row.id, cells: [labels[row.kind] || t("other"), labels[row.status] || t("other"), formatUsdMinor(row.amount_minor), row.available_at ? new Date(row.available_at).toLocaleDateString(locale) : "—"] }))} />
          </Card>
        </div>}
      </ListResourceView>
    </section>
  );
}
