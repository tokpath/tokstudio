"use client";

import { useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslations } from "next-intl";
import { z } from "zod";
import { LedgerTable } from "@/components/console/ledger-table";
import { ListResourceView } from "@/components/console/list-resource-view";
import { TextField } from "@/components/text-field";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Form } from "@/components/ui/form";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { fetchListItems } from "@/lib/list-resource";
import { USD_CREDIT, formatUsdMinor, parseUsdToMinor } from "@/lib/money";
import { ownerTypeLabelKey, statusLabelKey, statusTone } from "@/lib/status-copy";

type Plan = { id?: string; name?: string; status?: string; owner_type?: string; owner_id?: string; price_minor?: number; review_reason?: string };

export default function ChannelPlans() {
  const t = useTranslations("channelUi");
  const tc = useTranslations("common");
  const [createMessage, setCreateMessage] = useState(t("createHint"));
  const list = useListResource<Plan>({
    load: () => fetchListItems(`${apiBase}/channel/plans`),
  });
  const schema = useMemo(
    () =>
      z.object({
        name: z.string().trim().min(1, t("nameRequired")),
        price_usd: z.string().trim().min(1, t("priceRequired")),
        included_usd: z.string().trim().min(1, t("amountRequired")),
      }),
    [t],
  );
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { name: "", price_usd: "1", included_usd: "1" },
  });

  function statusText(status?: string) {
    const key = statusLabelKey(status);
    return key ? tc(key) : status ? tc("stUnknown", { status }) : "—";
  }

  function ownerText(owner?: string) {
    const key = ownerTypeLabelKey(owner);
    return key ? tc(key) : owner || "—";
  }

  return (
    <Card>
      <CardTitle className="mb-4 text-lg font-semibold tracking-tight">{t("plansTitle")}</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">{t("plansLead")}</p>
      <Button variant="outline" onClick={() => void list.reload()}>
        {t("refreshPlans")}
      </Button>
      <Form {...form}>
        <form
          className="mt-4 grid max-w-xl gap-2"
          onSubmit={form.handleSubmit(async (values) => {
            const price = parseUsdToMinor(values.price_usd);
            const included = parseUsdToMinor(values.included_usd);
            if (price == null || included == null) {
              setCreateMessage(t("priceRequired"));
              return;
            }
            const response = await fetch(`${apiBase}/channel/plans`, {
              method: "POST",
              credentials: "include",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({
                name: values.name,
                price_minor: price,
                items: [{ unit_type: USD_CREDIT, included_amount: included }],
              }),
            });
            const body = await response.json();
            if (!response.ok) {
              setCreateMessage(body.error?.message || tc("createFailed"));
              return;
            }
            form.reset();
            setCreateMessage(
              t("createdPlan", {
                id: body.item?.id || "",
                name: body.item?.name || values.name,
                status: `${statusText(body.item?.status)}${body.item?.review_reason ? ` (${body.item.review_reason})` : ""}`,
              }),
            );
            await list.reload();
          })}
        >
          <h3 className="text-lg font-medium">{t("createPlan")}</h3>
          <TextField control={form.control} name="name" label={t("planName")} />
          <TextField control={form.control} name="price_usd" label={t("planPrice")} placeholder={t("planPricePh")} suffix={tc("usd")} />
          <p className="text-sm text-ink-secondary">
            {t("planUnit")}：{t("planCreditUsd")}
          </p>
          <TextField control={form.control} name="included_usd" label={t("planAmount")} placeholder="1.00" suffix={tc("usd")} />
          <Button size="sm" type="submit">
            {t("createPlan")}
          </Button>
          <p className="text-sm text-ink-secondary">{createMessage}</p>
        </form>
      </Form>
      <ListResourceView
        snapshot={list.snapshot}
        loadingTitle={t("plansTitle")}
        emptyTitle={t("emptyPlans")}
        emptyDetail={t("emptyPlansDetail")}
        onRetry={() => void list.reload()}
      >
        <LedgerTable
          columns={[t("colPlan"), t("colStatus"), t("colOwner"), t("colPrice")]}
          emptyTitle={t("emptyPlans")}
          emptyDetail={t("emptyPlansDetail")}
          rows={list.snapshot.items.map((item) => ({
            key: item.id || item.name || "plan",
            cells: [
              item.name || "—",
              <Badge key="st" tone={statusTone(item.status)}>
                {statusText(item.status)}
              </Badge>,
              ownerText(item.owner_type),
              formatUsdMinor(item.price_minor, tc("lessThanCent")),
            ],
          }))}
        />
      </ListResourceView>
    </Card>
  );
}
