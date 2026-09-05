"use client";

import { useMemo, useState } from "react";
import { useTranslations } from "next-intl";
import { ActionRow } from "@/components/console/action-row";
import { Button } from "@/components/ui/button";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { apiBase } from "@/lib/api";
import type { CatalogModel } from "@/lib/catalog";

export function PlaygroundClient({ models }: { models: CatalogModel[] }) {
  const t = useTranslations("user");
  const fallbackId = models[0]?.id || "tokenhub/echo-1";
  const [model, setModel] = useState(fallbackId);
  const [prompt, setPrompt] = useState("");
  const [output, setOutput] = useState("");
  const [receiptModel, setReceiptModel] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState(t("pgHint"));

  const options = useMemo(() => (models.length ? models : [{ id: fallbackId, display_name: fallbackId, vendor: "" }]), [models, fallbackId]);

  async function send() {
    const text = prompt.trim();
    if (!text) {
      setMessage(t("pgNeedMsg"));
      return;
    }
    setBusy(true);
    setMessage(t("pgSending"));
    try {
      const response = await fetch(`${apiBase}/v1/chat/completions`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          model,
          messages: [{ role: "user", content: text }],
        }),
      });
      const body = await response.json();
      if (!response.ok) {
        setOutput("");
        setReceiptModel("");
        setMessage(body.error?.message || t("pgFail", { status: response.status }));
        return;
      }
      const content =
        body.choices?.[0]?.message?.content ||
        body.output_text ||
        JSON.stringify(body, null, 2);
      setOutput(typeof content === "string" ? content : JSON.stringify(content, null, 2));
      setReceiptModel(model);
      setMessage(t("pgDone"));
    } catch (err) {
      setMessage(err instanceof Error ? err.message : t("pgNet"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
      <section className="flex flex-col gap-3 rounded-card border border-hairline bg-canvas-raised p-5">
        <label className="flex flex-col gap-1 text-xs text-ink-mute">
          {t("pgModel")}
          <select
            aria-label={t("pgModelAria")}
            className="h-10 rounded-control border border-hairline bg-canvas px-3 text-sm text-ink"
            value={model}
            onChange={(e) => setModel(e.target.value)}
          >
            {options.map((item) => (
              <option key={item.id} value={item.id}>
                {item.display_name || item.id}
              </option>
            ))}
          </select>
        </label>
        <label className="flex flex-col gap-1 text-xs text-ink-mute">
          {t("pgMsg")}
          <textarea
            aria-label={t("pgMsgAria")}
            className="min-h-40 rounded-control border border-hairline bg-canvas px-3 py-2 text-sm text-ink"
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            placeholder={t("pgPh")}
          />
        </label>
        <ActionRow>
          <Button type="button" disabled={busy} onClick={() => void send()}>
            {t("pgSend")}
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() => {
              setPrompt("");
              setOutput("");
              setReceiptModel("");
              setMessage(t("pgCleared"));
            }}
          >
            {t("pgClear")}
          </Button>
        </ActionRow>
        <p className="text-sm text-ink-secondary">{message}</p>
      </section>
      <section className="rounded-card border border-hairline bg-canvas-raised p-5">
        <h2 className="mb-3 text-lg font-semibold">{t("pgReply")}</h2>
        {output ? (
          <div className="flex flex-col gap-3">
            {receiptModel ? (
              <aside
                className="rounded-stamp border border-hairline bg-canvas px-3 py-2 text-sm"
                aria-label={t("pgReceipt")}
              >
                <p className="th-eyebrow mb-2 text-ink-mute">{t("pgReceipt")}</p>
                <ul className="divide-y divide-hairline">
                  <li className="flex justify-between gap-3 py-1.5">
                    <span className="text-ink-mute">{t("pgReceiptModel")}</span>
                    <span className="font-mono text-ink">{receiptModel}</span>
                  </li>
                  <li className="flex justify-between gap-3 py-1.5">
                    <span className="text-ink-mute">{t("pgReceiptStatus")}</span>
                    <span className="text-ink">{t("pgReceiptOk")}</span>
                  </li>
                </ul>
              </aside>
            ) : null}
            <pre className="th-scrollbar max-h-[420px] overflow-auto whitespace-pre-wrap font-mono text-sm text-ink">{output}</pre>
          </div>
        ) : (
          <EmptyLedger title={t("pgEmpty")} detail={t("pgEmptyDetail")} />
        )}
      </section>
    </div>
  );
}
