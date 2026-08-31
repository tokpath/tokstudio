"use client";

import { useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslations } from "next-intl";
import { z } from "zod";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Form } from "@/components/ui/form";
import { apiBase } from "@/lib/api";
import { useToast } from "@/lib/toast";

export type APIKeyItem = {
  id: string;
  name: string;
  prefix: string;
  key?: string;
  status: string;
  rpm_limit?: number;
  concurrency_limit?: number;
  allowlist?: string[];
  expires_at?: string | null;
  last_used_at?: string | null;
};

export function parseAllowlist(raw: string): string[] {
  return raw
    .split(/[,，]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

export function KeysList({ items }: { items: APIKeyItem[] }) {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  if (items.length === 0) {
    return <EmptyLedger title={t("emptyKeys")} detail={t("emptyKeysDetail")} />;
  }
  return (
    <ul className="space-y-3 text-sm text-ink">
      {items.map((item) => (
        <li key={item.id} className="rounded-card border border-hairline bg-canvas p-3">
          <p>
            {item.name} · {item.prefix} · {item.status}
            {item.rpm_limit ? ` · RPM ${item.rpm_limit}` : ""}
            {item.concurrency_limit ? t("concurrency", { n: item.concurrency_limit }) : ""}
          </p>
          <p className="text-ink-secondary">
            {t("allowlistLine", { list: item.allowlist?.length ? item.allowlist.join(", ") : tc("unlimited") })}
          </p>
          {item.key ? <p className="break-all text-ink-secondary">{item.key}</p> : null}
        </li>
      ))}
    </ul>
  );
}

export default function KeysPanel() {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const [items, setItems] = useState<APIKeyItem[]>([]);
  const [createMessage, setCreateMessage] = useState(t("createHint"));
  const message = useToast((s) => s.message);
  const setMessage = useToast((s) => s.setMessage);
  const createSchema = useMemo(
    () =>
      z.object({
        name: z.string().trim().min(1, t("nameRequired")),
        allowlist: z.string(),
        rpm: z.string(),
        concurrency: z.string(),
      }),
    [t],
  );
  const form = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    defaultValues: { name: "default", allowlist: "", rpm: "", concurrency: "" },
  });

  async function refresh() {
    const response = await fetch(`${apiBase}/v1/me/api-keys`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || tc("notLoggedIn"));
      return;
    }
    setItems(body.items || []);
    setMessage(t("refreshed"));
  }

  async function createKey(values: z.infer<typeof createSchema>) {
    const models = parseAllowlist(values.allowlist);
    const payload: { name: string; allowlist?: string[]; rpm_limit?: number; concurrency_limit?: number } = { name: values.name };
    if (models.length > 0) {
      payload.allowlist = models;
    }
    const rpmLimit = Number(values.rpm);
    if (values.rpm && Number.isFinite(rpmLimit) && rpmLimit > 0) {
      payload.rpm_limit = rpmLimit;
    }
    const concLimit = Number(values.concurrency);
    if (values.concurrency && Number.isFinite(concLimit) && concLimit > 0) {
      payload.concurrency_limit = concLimit;
    }
    const response = await fetch(`${apiBase}/v1/me/api-keys`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    const body = await response.json();
    if (!response.ok) {
      setCreateMessage(body.error?.message || tc("createFailed"));
      return;
    }
    const created = body.item || {};
    const listed = Array.isArray(created.allowlist) && created.allowlist.length > 0 ? created.allowlist.join(", ") : tc("unlimited");
    setCreateMessage(t("created", { id: created.id || "", name: created.name || values.name, list: listed, n: created.concurrency_limit || 5 }));
    form.reset({ name: "default", allowlist: "", rpm: "", concurrency: "" });
    await refresh();
  }

  async function act(id: string, action: "rotate" | "disable" | "expire" | "copy") {
    const response = await fetch(`${apiBase}/v1/me/api-keys/${id}/${action}`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: action === "expire" ? JSON.stringify({ expires_at: new Date().toISOString() }) : "{}",
    });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || t("actFail"));
      return;
    }
    if (action === "copy" && typeof navigator !== "undefined") {
      const secret = items.find((item) => item.id === id)?.key;
      if (secret) {
        await navigator.clipboard.writeText(secret);
      }
    }
    setMessage(action === "copy" ? t("copiedAudit") : t("acted", { action }));
    await refresh();
  }

  return (
    <Card>
      <CardTitle>{t("keysTitle")}</CardTitle>
      <p className="mb-4 text-sm text-ink-secondary">{t("keysLead")}</p>
      <Form {...form}>
        <form className="mb-4 grid max-w-xl gap-3" onSubmit={form.handleSubmit(createKey)}>
          <TextField control={form.control} name="name" label={t("nameLabel")} placeholder={t("namePh")} />
          <h3 className="text-lg font-medium">{t("allowTitle")}</h3>
          <TextField control={form.control} name="allowlist" label={t("allowTitle")} placeholder={t("allowPh")} />
          <TextField control={form.control} name="rpm" label={t("rpm")} placeholder={t("rpmPh")} />
          <TextField control={form.control} name="concurrency" label={t("conc")} placeholder={t("concPh")} />
          <div className="flex flex-wrap gap-3">
            <Button type="submit">{tc("create")}</Button>
            <Button type="button" variant="outline" onClick={refresh}>
              {tc("refresh")}
            </Button>
          </div>
          <p className="text-sm text-ink-secondary">{createMessage}</p>
        </form>
      </Form>
      <KeysList items={items} />
      <ul className="mt-4 space-y-2 text-sm">
        {items.map((item) => (
          <li key={`${item.id}-actions`} className="flex flex-wrap gap-2">
            <Button size="sm" variant="outline" onClick={() => act(item.id, "copy")}>
              {tc("copy")}
            </Button>
            <Button size="sm" variant="outline" onClick={() => act(item.id, "rotate")}>
              {t("rotate")}
            </Button>
            <Button size="sm" variant="outline" onClick={() => act(item.id, "disable")}>
              {t("disable")}
            </Button>
            <Button size="sm" variant="outline" onClick={() => act(item.id, "expire")}>
              {t("expire")}
            </Button>
          </li>
        ))}
      </ul>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
