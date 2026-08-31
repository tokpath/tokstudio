"use client";

import { useState } from "react";
import { LedgerTable } from "@/components/console/ledger-table";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { apiBase } from "@/lib/api";
import { useTranslations } from "next-intl";

type PartnerMe = {
  role_type?: string;
  channel_org_id?: string;
  sees_downline?: boolean;
};

type PartnerUser = { email?: string; source_code?: string; status?: string };
type Commission = { id?: string; kind?: string; status?: string; amount_minor?: number };
type Settlement = { id?: string; status?: string; amount_minor?: number };

export type PartnerSection = "all" | "scope" | "users" | "commissions" | "settlements";

export function PartnerBoard({ section = "all" }: { section?: PartnerSection }) {
  const t = useTranslations("partnerBoard");
  const [me, setMe] = useState<PartnerMe>({});
  const [users, setUsers] = useState<PartnerUser[]>([]);
  const [comms, setComms] = useState<Commission[]>([]);
  const [settlements, setSettlements] = useState<Settlement[]>([]);
  const [message, setMessage] = useState(t("hint"));

  async function refresh() {
    const [meRes, userRes, commRes, setRes] = await Promise.all([
      fetch(`${apiBase}/v1/partner/me`, { credentials: "include" }),
      fetch(`${apiBase}/v1/partner/users`, { credentials: "include" }),
      fetch(`${apiBase}/v1/partner/commissions`, { credentials: "include" }),
      fetch(`${apiBase}/v1/partner/settlements`, { credentials: "include" }),
    ]);
    const meBody = await meRes.json();
    if (!meRes.ok) {
      setMessage(meBody.error?.message || t("notPartner"));
      return;
    }
    setMe(meBody as PartnerMe);
    const userBody = await userRes.json();
    const commBody = await commRes.json();
    const setBody = await setRes.json();
    setUsers((userBody.items || []) as PartnerUser[]);
    setComms((commBody.items || []) as Commission[]);
    setSettlements((setBody.items || []) as Settlement[]);
    setMessage(
      t("summary", {
        role: meBody.role_type || "—",
        users: userBody.items?.length ?? 0,
        comms: commBody.items?.length ?? 0,
        settlements: setBody.items?.length ?? 0,
      }),
    );
  }

  const show = (id: PartnerSection) => section === "all" || section === id;

  return (
    <div className="flex flex-col gap-6">
      {section !== "all" ? (
        <Button variant="outline" className="self-start" onClick={() => void refresh()}>
          {t("refresh")}
        </Button>
      ) : null}
      {show("scope") ? (
        <Card id="scope">
          <CardTitle>{t("hierarchy")}</CardTitle>
          <p className="mb-3 text-sm text-ink-secondary">
            {t("currentLine", {
              role: me.role_type || t("notLoggedIn"),
              channel: me.channel_org_id || "—",
              scope: me.sees_downline ? t("seesDownline") : t("seesDirect"),
            })}
          </p>
          <Button variant="outline" onClick={() => void refresh()}>
            {t("refresh")}
          </Button>
          <p className="mt-3 text-sm text-ink-secondary">{message}</p>
        </Card>
      ) : null}
      {show("users") ? (
        <Card id="users">
          <CardTitle>{t("users")}</CardTitle>
          <LedgerTable
            columns={[t("colEmail"), t("colCode"), t("colStatus")]}
            emptyTitle={t("emptyUsers")}
            emptyDetail={t("emptyUsersDetail")}
            rows={users.map((item) => ({
              key: `${item.email}-${item.source_code}`,
              cells: [item.email || "—", item.source_code || "—", item.status || "—"],
            }))}
          />
        </Card>
      ) : null}
      {show("commissions") ? (
        <Card id="commissions">
          <CardTitle>{t("commissions")}</CardTitle>
          <LedgerTable
            columns={[t("colKind"), t("colStatus"), t("colAmount")]}
            emptyTitle={t("emptyComms")}
            emptyDetail={t("emptyCommsDetail")}
            rows={comms.map((item) => ({
              key: item.id || `${item.kind}-${item.status}`,
              cells: [item.kind || "—", item.status || "—", `${item.amount_minor ?? 0} micro-USD`],
            }))}
          />
        </Card>
      ) : null}
      {show("settlements") ? (
        <Card id="settlements">
          <CardTitle>{t("settlements")}</CardTitle>
          <LedgerTable
            columns={[t("colSettle"), t("colStatus"), t("colAmount")]}
            emptyTitle={t("emptySettle")}
            emptyDetail={t("emptySettleDetail")}
            rows={settlements.map((item) => ({
              key: item.id || "settlement",
              cells: [item.id || "—", item.status || "—", `${item.amount_minor ?? 0} micro-USD`],
            }))}
          />
        </Card>
      ) : null}
    </div>
  );
}
