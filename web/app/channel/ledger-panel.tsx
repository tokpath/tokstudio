"use client";

import { useEffect, useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { ConfirmButton } from "@/components/confirm-button";
import { LedgerTable } from "@/components/console/ledger-table";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";

type Channel = { id?: string; code?: string; type?: string; parent_id?: string; status?: string };
type PnL = {
  recharge_minor?: number;
  unconsumed_minor?: number;
  consumed_minor?: number;
  marketing_minor?: number;
  marketing_frozen_minor?: number;
  marketing_issued_minor?: number;
  supplier_minor?: number;
  pnl_minor?: number;
};
type Supplier = {
  id?: string;
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

function newIdem() {
  return `spe-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

const selectClass =
  "h-10 min-h-10 w-full max-w-xs rounded-control border border-hairline bg-canvas-raised px-3 text-sm text-ink";

export default function ChannelLedger() {
  const t = useTranslations("channelUi");
  const tc = useTranslations("common");
  const [message, setMessage] = useState(t("ledgerHint"));
  const [channel, setChannel] = useState<Channel>({});
  const [pnl, setPnl] = useState<PnL>({});
  const [quota, setQuota] = useState<string>("—");
  const [entries, setEntries] = useState<Supplier[]>([]);
  const [children, setChildren] = useState<Channel[]>([]);
  const [usd, setUsd] = useState("10");
  const [sourceType, setSourceType] = useState("platform_recharge");
  const [vendor, setVendor] = useState("");
  const [memo, setMemo] = useState("");
  const [childID, setChildID] = useState("");
  const [wholesaleUsd, setWholesaleUsd] = useState("10");
  const [bCode, setBCode] = useState("");

  const isC = channel.type === "C";

  const defaultSource = useMemo(() => {
    if (channel.type === "C" || channel.type === "B") return "platform_recharge";
    return "provider_invoice";
  }, [channel.type]);

  async function refresh() {
    const meRes = await fetch(`${apiBase}/channel/me`, { credentials: "include" });
    const meBody = await meRes.json();
    if (!meRes.ok) {
      setMessage(meBody.error?.message || t("needAdmin"));
      return;
    }
    const id = String(meBody.channel_org_id || "");
    const [chRes, pnlRes, qRes, sRes, listRes] = await Promise.all([
      fetch(`${apiBase}/admin/channels/${encodeURIComponent(id)}`, { credentials: "include" }),
      fetch(`${apiBase}/channel/pnl`, { credentials: "include" }),
      fetch(`${apiBase}/channel/quota`, { credentials: "include" }),
      fetch(`${apiBase}/channel/supplier-entries`, { credentials: "include" }),
      fetch(`${apiBase}/admin/channels`, { credentials: "include" }),
    ]);
    const chBody = await chRes.json();
    const pnlBody = await pnlRes.json();
    const qBody = await qRes.json();
    const sBody = await sRes.json();
    const listBody = await listRes.json();
    if (!pnlRes.ok && !sRes.ok) {
      setMessage(pnlBody.error?.message || t("needAdmin"));
      return;
    }
    const next = (chBody.item || {}) as Channel;
    setChannel({ ...next, id: next.id || id });
    setPnl((pnlBody.pnl || {}) as PnL);
    setQuota(micro(qBody.quota?.available_minor));
    const items = Array.isArray(sBody.items) ? (sBody.items as Supplier[]) : [];
    setEntries(items);
    const kids = (Array.isArray(listBody.items) ? (listBody.items as Channel[]) : []).filter(
      (item) => item.parent_id === id && item.type === "B",
    );
    setChildren(kids);
    if (!childID && kids[0]?.id) setChildID(String(kids[0].id));
    setMessage(t("ledgerCount", { n: items.length, quota: micro(qBody.quota?.available_minor) }));
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    setSourceType(defaultSource);
  }, [defaultSource]);

  async function recordSupplier() {
    const amount = usdToMinor(usd);
    if (!amount) {
      setMessage(t("amountRequiredUsd"));
      return;
    }
    const res = await fetch(`${apiBase}/channel/supplier-entries`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({
        amount_minor: amount,
        source_type: sourceType,
        idempotency_key: newIdem(),
        vendor_name: vendor || undefined,
        memo: memo || undefined,
      }),
    });
    const body = await res.json();
    setMessage(res.ok ? t("recordedSupplier", { id: body.item?.id || "" }) : body.error?.message || t("needAdmin"));
    if (res.ok) await refresh();
  }

  async function reverse(id: string) {
    const res = await fetch(`${apiBase}/channel/supplier-entries/${encodeURIComponent(id)}/reverse`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({ reason: "void" }),
    });
    const body = await res.json();
    setMessage(res.ok ? t("reversedSupplier", { id: body.item?.id || "" }) : body.error?.message || t("needAdmin"));
    if (res.ok) await refresh();
  }

  async function wholesale() {
    const amount = usdToMinor(wholesaleUsd);
    if (!amount || !childID) {
      setMessage(t("wholesaleNeed"));
      return;
    }
    const res = await fetch(`${apiBase}/channel/quotas/grant`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({ channel_org_id: childID, amount_minor: amount }),
    });
    const body = await res.json();
    setMessage(res.ok ? t("wholesaleDone", { id: childID, left: micro(body.quota?.available_minor) }) : body.error?.message || t("needAdmin"));
    if (res.ok) await refresh();
  }

  async function createB() {
    const code = bCode.trim();
    if (!code) {
      setMessage(t("codeRequired"));
      return;
    }
    const res = await fetch(`${apiBase}/admin/channels`, {
      method: "POST",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({ code, type: "B", status: "active" }),
    });
    const body = await res.json();
    setMessage(res.ok ? t("createdB", { id: body.item?.id || "", code: body.item?.code || code }) : body.error?.message || t("needAdmin"));
    if (res.ok) {
      setBCode("");
      await refresh();
    }
  }

  const metrics = [
    { k: t("pnlRecharge"), v: micro(pnl.recharge_minor) },
    { k: t("pnlUnconsumed"), v: micro(pnl.unconsumed_minor) },
    { k: t("pnlConsumed"), v: micro(pnl.consumed_minor) },
    { k: t("pnlMarketing"), v: micro(pnl.marketing_minor) },
    { k: t("pnlSupplier"), v: micro(pnl.supplier_minor) },
    { k: t("pnlResult"), v: micro(pnl.pnl_minor) },
  ];

  return (
    <div className="flex flex-col gap-6">
      <Card>
        <CardTitle className="mb-4 text-lg font-semibold tracking-tight">{t("pnlTitle")}</CardTitle>
        <p className="mb-3 text-sm text-ink-secondary">{t("pnlLead", { quota })}</p>
        <dl className="mb-4 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {metrics.map((item) => (
            <div key={item.k} className="border-b border-hairline py-2">
              <dt className="text-sm text-ink-secondary">{item.k}</dt>
              <dd className="text-right font-mono text-lg tabular-nums">{item.v}</dd>
            </div>
          ))}
        </dl>
        <Button type="button" variant="outline" onClick={() => void refresh()}>
          {tc("refresh")}
        </Button>
      </Card>

      <Card>
        <CardTitle className="mb-4 text-lg font-semibold tracking-tight">{t("supplierTitle")}</CardTitle>
        <p className="mb-3 text-sm text-ink-secondary">{t("supplierLead")}</p>
        <div className="mb-3 grid max-w-xl gap-2">
          <Input value={usd} onChange={(e) => setUsd(e.target.value)} aria-label={t("usdLabel")} placeholder="10" />
          <select className={selectClass} aria-label={t("sourceLabel")} value={sourceType} onChange={(e) => setSourceType(e.target.value)}>
            <option value="platform_recharge">{t("sourcePlatform")}</option>
            <option value="provider_invoice">{t("sourceProvider")}</option>
            <option value="other">{t("sourceOther")}</option>
          </select>
          <Input value={vendor} onChange={(e) => setVendor(e.target.value)} aria-label={t("vendorLabel")} placeholder={t("vendorPh")} />
          <Input value={memo} onChange={(e) => setMemo(e.target.value)} aria-label={t("memoLabel")} placeholder={t("memoPh")} />
        </div>
        <ConfirmButton
          size="sm"
          title={t("confirmSupplier")}
          description={t("confirmSupplierD")}
          onConfirm={recordSupplier}
        >
          {t("recordSupplier")}
        </ConfirmButton>
        <LedgerTable
          columns={[t("colWhen"), t("colAmount"), t("colSource"), t("colMemo"), t("colAction")]}
          emptyTitle={t("emptySupplier")}
          emptyDetail={t("emptySupplierDetail")}
          rows={entries.map((item) => ({
            key: item.id || item.created_at || "spe",
            cells: [
              item.created_at ? String(item.created_at).slice(0, 19) : "—",
              <span key="a" className="font-mono tabular-nums">
                {micro(item.amount_minor)}
              </span>,
              item.source_type || "—",
              item.vendor_name || item.memo || item.reversal_of || "—",
              item.reversal_of ? (
                "—"
              ) : (
                <ConfirmButton
                  size="sm"
                  variant="outline"
                  title={t("confirmReverse")}
                  description={t("confirmReverseD")}
                  onConfirm={() => reverse(String(item.id))}
                >
                  {t("reverse")}
                </ConfirmButton>
              ),
            ],
          }))}
        />
      </Card>

      {isC ? (
        <>
          <Card>
            <CardTitle className="mb-4 text-lg font-semibold tracking-tight">{t("wholesaleTitle")}</CardTitle>
            <p className="mb-3 text-sm text-ink-secondary">{t("wholesaleLead")}</p>
            <div className="mb-3 flex flex-wrap gap-2">
              <select className={selectClass} aria-label={t("childLabel")} value={childID} onChange={(e) => setChildID(e.target.value)}>
                <option value="">{t("childNone")}</option>
                {children.map((item) => (
                  <option key={item.id} value={item.id}>
                    {item.code} ({item.id})
                  </option>
                ))}
              </select>
              <Input className="w-36" value={wholesaleUsd} onChange={(e) => setWholesaleUsd(e.target.value)} aria-label={t("usdLabel")} />
              <ConfirmButton size="sm" title={t("confirmWholesale")} description={t("confirmWholesaleD")} onConfirm={wholesale}>
                {t("grantWholesale")}
              </ConfirmButton>
            </div>
          </Card>
          <Card>
            <CardTitle className="mb-4 text-lg font-semibold tracking-tight">{t("createBTitle")}</CardTitle>
            <p className="mb-3 text-sm text-ink-secondary">{t("createBLead")}</p>
            <div className="mb-3 flex flex-wrap gap-2">
              <Input className="w-56" value={bCode} onChange={(e) => setBCode(e.target.value)} aria-label={t("bCodeLabel")} placeholder="THC-B1" />
              <ConfirmButton size="sm" title={t("confirmCreateB")} description={t("confirmCreateBD")} onConfirm={createB}>
                {t("createB")}
              </ConfirmButton>
            </div>
          </Card>
        </>
      ) : null}

      <p className="text-sm text-ink-secondary">{message}</p>
    </div>
  );
}
