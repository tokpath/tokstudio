"use client";

import { useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslations } from "next-intl";
import { z } from "zod";
import { ConfirmButton } from "@/components/confirm-button";
import { LedgerTable } from "@/components/console/ledger-table";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Form } from "@/components/ui/form";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";

type Promo = { id?: string; code?: string; status?: string; acquisition_role_id?: string };

export default function ChannelPromos() {
  const t = useTranslations("channelUi");
  const tc = useTranslations("common");
  const [items, setItems] = useState<Promo[]>([]);
  const [message, setMessage] = useState(t("promosHint"));
  const schema = useMemo(() => z.object({ code: z.string().trim().min(1, t("codeRequired")) }), [t]);
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { code: "" },
  });

  async function refresh() {
    const response = await fetch(`${apiBase}/channel/promotion-codes`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || t("needAdmin"));
      return;
    }
    const next = (body.items || []) as Promo[];
    setItems(next);
    setMessage(t("promosCount", { n: next.length }));
  }

  const origin = typeof window === "undefined" ? "" : window.location.origin;

  return (
    <Card>
      <CardTitle className="mb-3 text-xl font-medium">{t("promosTitle")}</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">{t("promosLead")}</p>
      <Form {...form}>
        <form className="mb-3 flex flex-wrap items-end gap-2" onSubmit={(event) => event.preventDefault()}>
          <TextField control={form.control} name="code" label={t("newCode")} placeholder={t("newCodePh")} showLabel={false} className="max-w-xs" />
          <ConfirmButton
            variant="outline"
            title={t("confirmPromo")}
            description={t("confirmPromoD")}
            validate={() => form.trigger()}
            onConfirm={form.handleSubmit(async (values) => {
              const response = await fetch(`${apiBase}/channel/promotion-codes`, {
                method: "POST",
                credentials: "include",
                headers: confirmHeaders,
                body: JSON.stringify({ code: values.code }),
              });
              const body = await response.json();
              if (!response.ok) {
                setMessage(body.error?.message || tc("createFailed"));
                return;
              }
              form.reset();
              await refresh();
              setMessage(t("createdPromo", { code: body.item?.code || "" }));
            })}
          >
            {t("createPromo")}
          </ConfirmButton>
          <Button variant="outline" onClick={refresh}>
            {t("refreshPromos")}
          </Button>
        </form>
      </Form>
      <LedgerTable
        columns={[t("colPromo"), t("colStatus"), t("colLink")]}
        emptyTitle={t("emptyPromos")}
        emptyDetail={t("emptyPromosDetail")}
        rows={items.map((item) => ({
          key: item.id || item.code || "promo",
          cells: [item.code || "—", item.status || "—", `${origin}/login?promo=${item.code}`],
        }))}
      />
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
