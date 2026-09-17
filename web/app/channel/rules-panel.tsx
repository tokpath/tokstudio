"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { ConfirmButton } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";

type Channel = { id?: string; type?: string };
type Policy = {
  version?: string;
  direct_bps?: number;
  indirect_bps?: number;
  total_bps?: number;
  freeze_days?: number;
  min_settle_minor?: number;
};
type Rule = {
  spend_minor?: number;
  topup_minor?: number;
  gift_minor?: number;
  inherited?: boolean;
};

function usdToMinor(raw: string) {
  const n = Number(raw);
  if (!Number.isFinite(n) || n < 0) return 0;
  return Math.round(n * 1_000_000);
}

function minorToUsd(n?: number) {
  if (n == null || !Number.isFinite(n)) return "0";
  return String(n / 1_000_000);
}

export default function ChannelRules() {
  const t = useTranslations("channelUi");
  const tc = useTranslations("common");
  const [message, setMessage] = useState(t("rulesHint"));
  const [ready, setReady] = useState(false);
  const [canWrite, setCanWrite] = useState(false);
  const [direct, setDirect] = useState("");
  const [indirect, setIndirect] = useState("");
  const [total, setTotal] = useState("");
  const [freeze, setFreeze] = useState("");
  const [minSettle, setMinSettle] = useState("");
  const [spend, setSpend] = useState("");
  const [topup, setTopup] = useState("");
  const [gift, setGift] = useState("");
  const [inherited, setInherited] = useState(false);

  async function refresh() {
    try {
      const meRes = await fetch(`${apiBase}/channel/me`, { credentials: "include" });
      const meBody = await meRes.json();
      if (!meRes.ok) {
        setMessage(meBody.error?.message || t("needAdmin"));
        return;
      }
      const id = String(meBody.channel_org_id || "");
      const [chRes, pRes, eRes] = await Promise.all([
        fetch(`${apiBase}/admin/channels/${encodeURIComponent(id)}`, { credentials: "include" }),
        fetch(`${apiBase}/channel/commission-policy`, { credentials: "include" }),
        fetch(`${apiBase}/channel/eligibility-rules`, { credentials: "include" }),
      ]);
      const chBody = await chRes.json();
      const pBody = await pRes.json();
      const eBody = await eRes.json();
      const ch = (chBody.item || {}) as Channel;
      setCanWrite(ch.type === "C");
      if (pRes.ok) {
        const p = (pBody.policy || {}) as Policy;
        setDirect(String(p.direct_bps ?? 0));
        setIndirect(String(p.indirect_bps ?? 0));
        setTotal(String(p.total_bps ?? 0));
        setFreeze(String(p.freeze_days ?? 0));
        setMinSettle(minorToUsd(p.min_settle_minor ?? 0));
      }
      if (eRes.ok) {
        const r = (eBody.rule || {}) as Rule;
        setSpend(minorToUsd(r.spend_minor));
        setTopup(minorToUsd(r.topup_minor));
        setGift(minorToUsd(r.gift_minor));
        setInherited(Boolean(r.inherited));
      }
      if (!pRes.ok && !eRes.ok) {
        setMessage(pBody.error?.message || t("needAdmin"));
        return;
      }
      setMessage(ch.type === "C" ? t("rulesWriteHint") : t("rulesReadHint"));
    } finally {
      setReady(true);
    }
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function savePolicy(): Promise<boolean> {
    try {
    const res = await fetch(`${apiBase}/channel/commission-policy`, {
      method: "PATCH",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({
        direct_bps: Number(direct),
        indirect_bps: Number(indirect),
        total_bps: Number(total),
        freeze_days: Number(freeze),
        min_settle_minor: usdToMinor(minSettle),
      }),
    });
    const body = await res.json();
    setMessage(res.ok ? t("savedPolicy", { v: body.policy?.version || "" }) : body.error?.message || t("needAdmin"));
    const __ok = res.ok;
    if (res.ok) await refresh();
    return __ok;
    } catch {
      setMessage(tc("listNetwork"));
      return false;
    }
}

  async function saveEligibility(): Promise<boolean> {
    try {
    const res = await fetch(`${apiBase}/channel/eligibility-rules`, {
      method: "PATCH",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({
        spend_minor: usdToMinor(spend),
        topup_minor: usdToMinor(topup),
        gift_minor: usdToMinor(gift),
      }),
    });
    const body = await res.json();
    setMessage(res.ok ? t("savedEligibility") : body.error?.message || t("needAdmin"));
    const __ok = res.ok;
    if (res.ok) await refresh();
    return __ok;
    } catch {
      setMessage(tc("listNetwork"));
      return false;
    }
}

  if (!ready) {
    return <p className="text-sm text-ink-secondary">{t("rulesLoading")}</p>;
  }

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardTitle className="mb-4 text-lg font-semibold tracking-tight">{t("policyTitle")}</CardTitle>
        <p className="mb-3 text-sm text-ink-secondary">{t("policyLead")}</p>
        <div className="mb-3 grid max-w-3xl grid-cols-2 gap-2 md:grid-cols-3">
          <Input value={direct} onChange={(e) => setDirect(e.target.value)} aria-label={t("directBps")} />
          <Input value={indirect} onChange={(e) => setIndirect(e.target.value)} aria-label={t("indirectBps")} />
          <Input value={total} onChange={(e) => setTotal(e.target.value)} aria-label={t("totalBps")} />
          <Input value={freeze} onChange={(e) => setFreeze(e.target.value)} aria-label={t("freezeDays")} />
          <Input value={minSettle} onChange={(e) => setMinSettle(e.target.value)} aria-label={t("minSettleUsd")} />
        </div>
        <div className="flex flex-wrap gap-2">
          <Button type="button" size="sm" variant="outline" onClick={() => void refresh()}>
            {tc("refresh")}
          </Button>
          {canWrite ? (
            <ConfirmButton size="sm" title={t("confirmPolicy")} description={t("confirmPolicyD")} onConfirm={savePolicy}>
              {t("savePolicy")}
            </ConfirmButton>
          ) : null}
        </div>
      </Card>
      <Card>
        <CardTitle className="mb-4 text-lg font-semibold tracking-tight">{t("eligTitle")}</CardTitle>
        <p className="mb-3 text-sm text-ink-secondary">{inherited ? t("eligInherited") : t("eligLead")}</p>
        <div className="mb-3 grid max-w-3xl grid-cols-2 gap-2 md:grid-cols-3">
          <Input value={spend} onChange={(e) => setSpend(e.target.value)} aria-label={t("spendUsd")} />
          <Input value={topup} onChange={(e) => setTopup(e.target.value)} aria-label={t("topupUsd")} />
          <Input value={gift} onChange={(e) => setGift(e.target.value)} aria-label={t("giftUsd")} />
        </div>
        {canWrite ? (
          <ConfirmButton size="sm" title={t("confirmElig")} description={t("confirmEligD")} onConfirm={saveEligibility}>
            {t("saveElig")}
          </ConfirmButton>
        ) : null}
      </Card>
      <p className="text-sm text-ink-secondary">{message}</p>
    </div>
  );
}
