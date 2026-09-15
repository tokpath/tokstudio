"use client";

import { useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslations } from "next-intl";
import { z } from "zod";
import { ConfirmButton } from "@/components/confirm-button";
import { LedgerTable } from "@/components/console/ledger-table";
import { ListResourceView } from "@/components/console/list-resource-view";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Form } from "@/components/ui/form";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { fetchListItems } from "@/lib/list-resource";

type Promo = { id?: string; code?: string; status?: string; acquisition_role_id?: string };

export default function ChannelPromos() {
  const t = useTranslations("channelUi");
  const tc = useTranslations("common");
  const [message, setMessage] = useState(t("promosHint"));
  const list = useListResource<Promo>({
    load: () => fetchListItems(`${apiBase}/channel/promotion-codes`),
  });
  const schema = useMemo(() => z.object({ code: z.string().trim().min(1, t("codeRequired")) }), [t]);
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { code: "" },
  });

  const origin = typeof window === "undefined" ? "" : window.location.origin;

  return (
    <Card>
      <CardTitle className="mb-4 text-lg font-semibold tracking-tight">{t("promosTitle")}</CardTitle>
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
              await list.reload();
              setMessage(t("createdPromo", { code: body.item?.code || "" }));
            })}
          >
            {t("createPromo")}
          </ConfirmButton>
          <Button variant="outline" onClick={() => void list.reload()}>
            {t("refreshPromos")}
          </Button>
        </form>
      </Form>
      <ListResourceView
        snapshot={list.snapshot}
        emptyTitle={t("emptyPromos")}
        emptyDetail={t("emptyPromosDetail")}
        onRetry={() => void list.reload()}
      >
        <LedgerTable
          columns={[t("colPromo"), t("colStatus"), t("colLink")]}
          emptyTitle={t("emptyPromos")}
          emptyDetail={t("emptyPromosDetail")}
          rows={list.snapshot.items.map((item) => ({
            key: item.id || item.code || "promo",
            cells: [item.code || "—", item.status || "—", `${origin}/login?promo=${item.code}`],
          }))}
        />
      </ListResourceView>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
