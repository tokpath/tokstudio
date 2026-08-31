"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";

type Balance = {
  available?: string;
  reserved?: string;
  available_minor?: number;
  reserved_minor?: number;
};

export default function WalletPanel() {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const [balance, setBalance] = useState<Balance | null>(null);
  const [code, setCode] = useState("THE2E");
  const [message, setMessage] = useState(t("walletHint"));

  async function refresh() {
    const response = await fetch(`${apiBase}/v1/me/balance`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || tc("notLoggedIn"));
      return;
    }
    setBalance(body.balance);
    setMessage(t("walletRefreshed"));
  }

  async function redeem() {
    const response = await fetch(`${apiBase}/v1/topups/redeem`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code }),
    });
    const body = await response.json();
    setMessage(response.ok ? t("redeemOk", { amount: body.item?.amount_minor }) : body.error?.message || t("redeemFail"));
    if (response.ok) {
      await refresh();
    }
  }

  return (
    <section className="rounded-card border border-hairline bg-canvas-raised p-6 ">
      <h2 className="mb-3 text-xl font-medium tracking-tight">{t("walletTitle")}</h2>
      <p className="mb-4 text-sm text-ink-secondary">
        {t("walletMeta", { available: balance?.available ?? "—", reserved: balance?.reserved ?? "0" })}
      </p>
      <div className="flex flex-wrap gap-3">
        <Button type="button" variant="outline" onClick={refresh}>
          {t("refreshBalance")}
        </Button>
        <Input aria-label={t("redeemCode")} className="max-w-xs" value={code} onChange={(e) => setCode(e.target.value)} />
        <Button type="button" onClick={redeem}>
          {t("redeem")}
        </Button>
      </div>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </section>
  );
}
