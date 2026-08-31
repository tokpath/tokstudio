"use client";

import { useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslations } from "next-intl";
import { z } from "zod";
import { LedgerTable } from "@/components/console/ledger-table";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Form } from "@/components/ui/form";
import { apiBase } from "@/lib/api";

type Plan = { id?: string; name?: string; status?: string; owner_type?: string; owner_id?: string; price_minor?: number; review_reason?: string };

export default function ChannelPlans() {
  const t = useTranslations("channelUi");
  const tc = useTranslations("common");
  const [items, setItems] = useState<Plan[]>([]);
  const [message, setMessage] = useState(t("plansHint"));
  const [createMessage, setCreateMessage] = useState(t("createHint"));
  const schema = useMemo(
    () =>
      z.object({
        name: z.string().trim().min(1, t("nameRequired")),
        price_minor: z.string().trim().min(1, t("priceRequired")),
        unit_type: z.string().trim().min(1, t("unitRequired")),
        included_amount: z.string().trim().min(1, t("amountRequired")),
      }),
    [t],
  );
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { name: "", price_minor: "1000", unit_type: "usd_credit", included_amount: "1" },
  });

  async function refresh() {
    const response = await fetch(`${apiBase}/channel/plans`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || t("needAdmin"));
      return;
    }
    const next = (body.items || []) as Plan[];
    setItems(next);
    setMessage(t("plansCount", { n: next.length }));
  }

  return (
    <Card className="rounded-card border border-hairline bg-canvas-raised  p-6">
      <CardTitle className="mb-3 text-xl font-medium">{t("plansTitle")}</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">{t("plansLead")}</p>
      <Button variant="outline" onClick={refresh}>
        {t("refreshPlans")}
      </Button>
      <Form {...form}>
        <form
          className="mt-4 grid max-w-xl gap-2"
          onSubmit={form.handleSubmit(async (values) => {
            const response = await fetch(`${apiBase}/channel/plans`, {
              method: "POST",
              credentials: "include",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({
                name: values.name,
                price_minor: Number(values.price_minor || 0),
                items: [
                  {
                    unit_type: values.unit_type || "usd_credit",
                    included_amount: Number(values.included_amount || 0),
                  },
                ],
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
                status: `${body.item?.status || ""}${body.item?.review_reason ? ` (${body.item.review_reason})` : ""}`,
              }),
            );
            await refresh();
          })}
        >
          <h3 className="text-lg font-medium">{t("createPlan")}</h3>
          <TextField control={form.control} name="name" label={t("planName")} />
          <TextField control={form.control} name="price_minor" label={t("planPrice")} placeholder={t("planPricePh")} />
          <TextField control={form.control} name="unit_type" label={t("planUnit")} placeholder="usd_credit" />
          <TextField control={form.control} name="included_amount" label={t("planAmount")} placeholder="included_amount" />
          <Button size="sm" type="submit">
            {t("createPlan")}
          </Button>
          <p className="text-sm text-ink-secondary">{createMessage}</p>
        </form>
      </Form>
      <LedgerTable
        columns={[t("colPlan"), t("colStatus"), t("colOwner"), t("colPrice")]}
        emptyTitle={t("emptyPlans")}
        emptyDetail={t("emptyPlansDetail")}
        rows={items.map((item) => ({
          key: item.id || item.name || "plan",
          cells: [item.name || "—", item.status || "—", item.owner_type || "—", `${item.price_minor ?? 0} micro-USD`],
        }))}
      />
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
