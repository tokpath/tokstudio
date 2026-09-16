"use client";

import { LedgerTable } from "@/components/console/ledger-table";
import { ListResourceView } from "@/components/console/list-resource-view";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { fetchListItems } from "@/lib/list-resource";
import { formatUsdMinor } from "@/lib/money";
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
  const users = useListResource<PartnerUser>({
    enabled: section === "all" || section === "users",
    load: () => fetchListItems(`${apiBase}/v1/partner/users`),
  });
  const comms = useListResource<Commission>({
    enabled: section === "all" || section === "commissions",
    load: () => fetchListItems(`${apiBase}/v1/partner/commissions`),
  });
  const settlements = useListResource<Settlement>({
    enabled: section === "all" || section === "settlements",
    load: () => fetchListItems(`${apiBase}/v1/partner/settlements`),
  });
  const me = useListResource<PartnerMe>({
    enabled: section === "all" || section === "scope",
    load: async () => {
      try {
        const response = await fetch(`${apiBase}/v1/partner/me`, { credentials: "include" });
        const body = await response.json().catch(() => ({}));
        if (!response.ok) {
          return { ok: false, status: response.status, items: [], message: body.error?.message, code: body.error?.code };
        }
        return { ok: true, status: response.status, items: [body as PartnerMe] };
      } catch {
        return { ok: false, network: true, items: [] };
      }
    },
  });
  const profile = me.snapshot.items[0] || {};

  function refreshAll() {
    void me.reload();
    void users.reload();
    void comms.reload();
    void settlements.reload();
  }

  const show = (id: PartnerSection) => section === "all" || section === id;

  return (
    <div className="flex flex-col gap-6">
      {section !== "all" ? (
        <Button variant="outline" className="self-start" onClick={refreshAll}>
          {t("refresh")}
        </Button>
      ) : null}
      {show("scope") ? (
        <Card id="scope">
          <CardTitle>{t("hierarchy")}</CardTitle>
          {me.snapshot.phase === "ready" ? (
            <p className="mb-3 text-sm text-ink-secondary">
              {t("currentLine", {
                role: profile.role_type || t("notLoggedIn"),
                channel: profile.channel_org_id || "—",
                scope: profile.sees_downline ? t("seesDownline") : t("seesDirect"),
              })}
            </p>
          ) : null}
          <Button variant="outline" onClick={refreshAll}>
            {t("refresh")}
          </Button>
          <ListResourceView snapshot={me.snapshot} emptyTitle={t("hierarchy")} emptyDetail={t("hint")} onRetry={() => void me.reload()}>
            <p className="mt-3 text-sm text-ink-secondary">{t("hint")}</p>
          </ListResourceView>
        </Card>
      ) : null}
      {show("users") ? (
        <Card id="users">
          <CardTitle>{t("users")}</CardTitle>
          <ListResourceView
            snapshot={users.snapshot}
            emptyTitle={t("emptyUsers")}
            emptyDetail={t("emptyUsersDetail")}
            onRetry={() => void users.reload()}
          >
            <LedgerTable
              columns={[t("colEmail"), t("colCode"), t("colStatus")]}
              emptyTitle={t("emptyUsers")}
              emptyDetail={t("emptyUsersDetail")}
              rows={users.snapshot.items.map((item) => ({
                key: `${item.email}-${item.source_code}`,
                cells: [item.email || "—", item.source_code || "—", item.status || "—"],
              }))}
            />
          </ListResourceView>
        </Card>
      ) : null}
      {show("commissions") ? (
        <Card id="commissions">
          <CardTitle>{t("commissions")}</CardTitle>
          <ListResourceView
            snapshot={comms.snapshot}
            emptyTitle={t("emptyComms")}
            emptyDetail={t("emptyCommsDetail")}
            onRetry={() => void comms.reload()}
          >
            <LedgerTable
              columns={[t("colKind"), t("colStatus"), t("colAmount")]}
              emptyTitle={t("emptyComms")}
              emptyDetail={t("emptyCommsDetail")}
              rows={comms.snapshot.items.map((item) => ({
                key: item.id || `${item.kind}-${item.status}`,
                cells: [item.kind || "—", item.status || "—", formatUsdMinor(item.amount_minor)],
              }))}
            />
          </ListResourceView>
        </Card>
      ) : null}
      {show("settlements") ? (
        <Card id="settlements">
          <CardTitle>{t("settlements")}</CardTitle>
          <ListResourceView
            snapshot={settlements.snapshot}
            emptyTitle={t("emptySettle")}
            emptyDetail={t("emptySettleDetail")}
            onRetry={() => void settlements.reload()}
          >
            <LedgerTable
              columns={[t("colSettle"), t("colStatus"), t("colAmount")]}
              emptyTitle={t("emptySettle")}
              emptyDetail={t("emptySettleDetail")}
              rows={settlements.snapshot.items.map((item) => ({
                key: item.id || "settlement",
                cells: [item.id || "—", item.status || "—", formatUsdMinor(item.amount_minor)],
              }))}
            />
          </ListResourceView>
        </Card>
      ) : null}
    </div>
  );
}
