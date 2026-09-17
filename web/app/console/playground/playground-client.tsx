"use client";

import { useMemo, useRef, useState } from "react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { ActionRow } from "@/components/console/action-row";
import { Button } from "@/components/ui/button";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { apiBase } from "@/lib/api";
import type { CatalogModel } from "@/lib/catalog";
import { pickPlaygroundModel } from "@/lib/playground-session";
import { catalogModelUsable, modelEntry, useModelHref } from "@/lib/model-use";
import { copyText } from "@/lib/submit-result";

type Turn = {
  id: string;
  prompt: string;
  reply: string;
  model: string;
  status: "ok" | "fail" | "pending";
  error?: string;
};

type ChatMessage = { role: "user" | "assistant"; content: string };

function extractReply(body: Record<string, unknown>): string {
  const choices = body.choices as { message?: { content?: unknown } }[] | undefined;
  const content = choices?.[0]?.message?.content ?? body.output_text ?? body;
  return typeof content === "string" ? content : JSON.stringify(content, null, 2);
}

export function PlaygroundClient({
  models,
  initialModel,
  catalogHref = "/app/catalog",
  catalogOk = true,
  catalogMessage,
}: {
  models: CatalogModel[];
  initialModel?: string;
  catalogHref?: string;
  catalogOk?: boolean;
  catalogMessage?: string;
}) {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const pick = useMemo(() => pickPlaygroundModel(models, initialModel), [models, initialModel]);
  const [model, setModel] = useState(pick.id);
  const [prompt, setPrompt] = useState("");
  const [turns, setTurns] = useState<Turn[]>([]);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState(t("pgHint"));
  const abortRef = useRef<AbortController | null>(null);

  const options = useMemo(
    () => models.filter((item) => catalogModelUsable(item) && modelEntry(item) === "chat"),
    [models],
  );

  const examples = [t("pgEx0"), t("pgEx1"), t("pgEx2")];

  function stopWait() {
    abortRef.current?.abort();
  }

  function newChat() {
    abortRef.current?.abort();
    setMessages([]);
    setMessage(t("pgNewChatReady"));
  }

  async function send(reset: boolean) {
    const text = prompt.trim();
    if (!text) {
      setMessage(t("pgNeedMsg"));
      return;
    }
    if (!options.some((item) => item.id === model)) {
      setMessage(t("pgModelMissing"));
      return;
    }
    const pending: Turn = {
      id: `${Date.now()}`,
      prompt: text,
      reply: "",
      model,
      status: "pending",
    };
    const history = reset ? [] : messages;
    const payload = [...history, { role: "user" as const, content: text }];
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;
    setTurns((current) => [...current, pending]);
    setBusy(true);
    setMessage(t("pgWaiting"));
    try {
      const response = await fetch(`${apiBase}/v1/chat/completions`, {
        method: "POST",
        credentials: "include",
        signal: controller.signal,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ model, messages: payload }),
      });
      const body = (await response.json()) as Record<string, unknown> & { error?: { message?: string } };
      if (!response.ok) {
        const error = body.error?.message || t("pgFail", { status: response.status });
        setTurns((current) =>
          current.map((item) => (item.id === pending.id ? { ...item, status: "fail", error } : item)),
        );
        setMessage(error);
        return;
      }
      const reply = extractReply(body);
      setTurns((current) =>
        current.map((item) => (item.id === pending.id ? { ...item, status: "ok", reply } : item)),
      );
      setMessages([...payload, { role: "assistant", content: reply }]);
      setPrompt("");
      setMessage(t("pgDone"));
    } catch (err) {
      if (err instanceof Error && err.name === "AbortError") {
        setTurns((current) => current.filter((item) => item.id !== pending.id));
        setMessage(t("pgStopped"));
        return;
      }
      const error = err instanceof Error ? err.message : t("pgNet");
      setTurns((current) =>
        current.map((item) => (item.id === pending.id ? { ...item, status: "fail", error } : item)),
      );
      setMessage(error);
    } finally {
      setBusy(false);
      if (abortRef.current === controller) {
        abortRef.current = null;
      }
    }
  }

  async function copyLast() {
    const last = [...turns].reverse().find((item) => item.status === "ok" && item.reply);
    if (!last) {
      return;
    }
    const wrote = await copyText(last.reply);
    setMessage(wrote ? t("pgCopied") : t("pgCopyFailed"));
  }

  if (!catalogOk) {
    return (
      <div className="space-y-3" data-testid="catalog-status" data-catalog-ok="false" role="alert">
        <Button asChild variant="outline" size="sm">
          <Link href={catalogHref}>{t("pgBackCatalog")}</Link>
        </Button>
        <EmptyLedger title={tc("listFailed")} detail={catalogMessage || tc("listNetwork")} />
      </div>
    );
  }

  if (pick.error === "missing") {
    return (
      <div className="space-y-3" data-testid="model-entry-error" data-reason="missing" role="alert">
        <Button asChild variant="outline" size="sm">
          <Link href={catalogHref}>{t("pgBackCatalog")}</Link>
        </Button>
        <EmptyLedger title={t("pgModelMissing")} detail={t("pgModelMissingDetail")} />
      </div>
    );
  }

  if (pick.error === "unavailable") {
    return (
      <div className="space-y-3" data-testid="model-entry-error" data-reason="unavailable" role="alert">
        <Button asChild variant="outline" size="sm">
          <Link href={catalogHref}>{t("pgBackCatalog")}</Link>
        </Button>
        <EmptyLedger title={t("pgModelUnavailable")} detail={t("pgModelUnavailableDetail", { id: pick.id })} />
      </div>
    );
  }

  if (pick.error === "notChat") {
    const found = models.find((item) => item.id === pick.id);
    const href = found ? useModelHref(found, catalogHref) : catalogHref;
    return (
      <div className="space-y-3" data-testid="model-entry-error" data-reason="notChat" role="alert">
        <Button asChild variant="outline" size="sm">
          <Link href={catalogHref}>{t("pgBackCatalog")}</Link>
        </Button>
        <EmptyLedger title={t("pgModelNotChat")} detail={t("pgModelNotChatDetail")} />
        <Button asChild size="sm">
          <Link href={href}>{t("pgOpenMatching")}</Link>
        </Button>
      </div>
    );
  }

  if (options.length === 0) {
    return (
      <div className="space-y-3">
        <Button asChild variant="outline" size="sm">
          <Link href={catalogHref}>{t("pgBackCatalog")}</Link>
        </Button>
        <EmptyLedger title={t("pgNoModels")} detail={t("pgNoModelsDetail")} />
      </div>
    );
  }

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
      <section className="flex flex-col gap-3 rounded-card border border-hairline bg-canvas-raised p-5">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <Button asChild variant="outline" size="sm">
            <Link href={catalogHref}>{t("pgBackCatalog")}</Link>
          </Button>
        </div>
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
        <div className="flex flex-wrap gap-1.5" aria-label={t("pgExamples")}>
          {examples.map((item) => (
            <button
              key={item}
              type="button"
              className="rounded-control border border-hairline px-2.5 py-1 text-xs text-ink-secondary hover:bg-canvas hover:text-ink"
              onClick={() => setPrompt(item)}
            >
              {item}
            </button>
          ))}
        </div>
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
          <Button type="button" disabled={busy || !model} onClick={() => void send(messages.length === 0)}>
            {busy ? t("pgWaiting") : messages.length ? t("pgContinue") : t("pgSend")}
          </Button>
          {messages.length > 0 ? (
            <Button type="button" variant="outline" disabled={busy} onClick={newChat}>
              {t("pgNewChat")}
            </Button>
          ) : null}
          {busy ? (
            <Button type="button" variant="ghost" onClick={stopWait}>
              {t("pgStopWait")}
            </Button>
          ) : null}
          <Button type="button" variant="outline" disabled={busy || !turns.some((item) => item.status === "ok")} onClick={() => void copyLast()}>
            {t("pgCopy")}
          </Button>
          <Button
            type="button"
            variant="ghost"
            disabled={busy || !prompt.trim()}
            onClick={() => void send(messages.length === 0)}
          >
            {t("pgRetry")}
          </Button>
        </ActionRow>
        <p className="text-sm text-ink-secondary">{message}</p>
        <p className="text-[12px] text-ink-mute">{t("pgStopHint")}</p>
      </section>
      <section className="rounded-card border border-hairline bg-canvas-raised p-5">
        <h2 className="mb-3 text-lg font-semibold">{t("pgReply")}</h2>
        {turns.length === 0 ? (
          <EmptyLedger title={t("pgEmpty")} detail={t("pgEmptyDetail")} />
        ) : (
          <ol className="flex flex-col gap-3">
            {turns.map((item) => (
              <li key={item.id} className="rounded-control border border-hairline bg-canvas px-3 py-3 text-sm">
                <p className="text-ink">{item.prompt}</p>
                {item.status === "pending" ? (
                  <p className="mt-2 text-ink-mute">{t("pgWaiting")}</p>
                ) : item.status === "fail" ? (
                  <p className="mt-2 text-danger">{item.error}</p>
                ) : (
                  <pre className="th-scrollbar mt-2 max-h-60 overflow-auto whitespace-pre-wrap font-mono text-[13px] text-ink">
                    {item.reply}
                  </pre>
                )}
                <p className="mt-2 font-mono text-[11px] text-ink-mute">{item.model}</p>
              </li>
            ))}
          </ol>
        )}
      </section>
    </div>
  );
}
