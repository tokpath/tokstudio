"use client";

import { useMemo, useRef, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslations } from "next-intl";
import { z } from "zod";
import { LedgerTable } from "@/components/console/ledger-table";
import { ListResourceView } from "@/components/console/list-resource-view";
import { SubmitStatus } from "@/components/console/submit-status";
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
import { confirmJsonAction } from "@/lib/submit-result";

type Plan = { id?: string; name?: string; status?: string; owner_type?: string; owner_id?: string; price_minor?: number; review_reason?: string };

export function ChannelPlans() {
  const t = useTranslations("channelUi");
  const tc = useTranslations("common");
  const [createMessage, setCreateMessage] = useState(t("createHint"));
  const [createError, setCreateError] = useState("");
  const [creating, setCreating] = useState(false);
  const creatingRef = useRef(false);
  const submitGen = useRef(0);
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

  async function createPlan(values: z.infer<typeof schema>) {
    if (creatingRef.current) {
      return;
    }
    const price = parseUsdToMinor(values.price_usd);
    const included = parseUsdToMinor(values.included_usd);
    if (price == null || included == null) {
      setCreateError(t("priceRequired"));
      return;
    }
    const generation = ++submitGen.current;
    creatingRef.current = true;
    setCreating(true);
    setCreateError("");
    try {
      return await confirmJsonAction({
        request: () =>
          fetch(`${apiBase}/channel/plans`, {
            method: "POST",
            credentials: "include",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              name: values.name,
              price_minor: price,
              items: [{ unit_type: USD_CREDIT, included_amount: included }],
            }),
          }),
        failFallback: tc("createFailed"),
        networkMessage: tc("listNetwork"),
        onError: (message) => {
          if (generation !== submitGen.current) {
            return;
          }
          setCreateError(message);
        },
        onSuccess: async (body) => {
          if (generation !== submitGen.current) {
            return;
          }
          const item = (body as { item?: Plan }).item;
          form.reset();
          setCreateError("");
          setCreateMessage(
            t("createdPlan", {
              id: item?.id || "",
              name: item?.name || values.name,
              status: `${statusText(item?.status)}${item?.review_reason ? ` (${item.review_reason})` : ""}`,
            }),
          );
          await list.reload();
        },
      });
    } finally {
      creatingRef.current = false;
      setCreating(false);
    }
  }

  return (
    <Card>
      <CardTitle className="mb-4 text-lg font-semibold tracking-tight">{t("plansTitle")}</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">{t("plansLead")}</p>
      <Button variant="outline" onClick={() => void list.reload()}>
        {t("refreshPlans")}
      </Button>
      <Form {...form}>
        <form className="mt-4 grid max-w-xl gap-2" onSubmit={form.handleSubmit((values) => void createPlan(values))}>
          <h3 className="text-lg font-medium">{t("createPlan")}</h3>
          <TextField control={form.control} name="name" label={t("planName")} />
          <TextField control={form.control} name="price_usd" label={t("planPrice")} placeholder={t("planPricePh")} suffix={tc("usd")} />
          <p className="text-sm text-ink-secondary">
            {t("planUnit")}：{t("planCreditUsd")}
          </p>
          <TextField control={form.control} name="included_usd" label={t("planAmount")} placeholder="1.00" suffix={tc("usd")} />
          <Button size="sm" type="submit" disabled={creating}>
            {creating ? tc("submitting") : t("createPlan")}
          </Button>
          {createError ? <SubmitStatus error={createError} /> : <p className="text-sm text-ink-secondary">{createMessage}</p>}
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

export default ChannelPlans;
