"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { ActionRow } from "@/components/console/action-row";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { apiBase } from "@/lib/api";

type DocsContext = {
  api_base_url?: string;
  brand?: { name?: string; api_domain?: string };
  models?: string[];
  examples?: { curl?: string; python?: string; node?: string; messages?: string; video?: string };
  notes?: { auth?: string; errors?: string; rate_limit?: string; webhook?: string };
};

type ExampleTab = "curl" | "python" | "node";

type Snippet = { id: ExampleTab; copyLabel: string; copyButton: string; text?: string };

export default function ExamplesPanel({
  catalogHref = "/app/catalog",
  catalogOk = true,
  catalogMessage,
  requestedModel,
  modelError,
  focusCurl,
  focusPath,
}: {
  catalogHref?: string;
  catalogOk?: boolean;
  catalogMessage?: string;
  requestedModel?: string;
  modelError?: "missing" | "unavailable";
  focusCurl?: string;
  focusPath?: string;
} = {}) {
  const t = useTranslations("user");
  const [docs, setDocs] = useState<DocsContext>({});
  const [tab, setTab] = useState<ExampleTab>("curl");
  const [message, setMessage] = useState(t("examplesHint"));

  async function refresh() {
    const host = typeof window !== "undefined" ? window.location.host : "localhost";
    const response = await fetch(`${apiBase}/v1/public/docs-context?host=${encodeURIComponent(host)}`, {
      credentials: "include",
    });
    const body = (await response.json()) as DocsContext & { error?: { message?: string } };
    if (!response.ok) {
      setMessage(body.error?.message || t("examplesFail"));
      return;
    }
    setDocs(body);
    setMessage(t("examplesMeta", { host: body.api_base_url || `https://${body.brand?.api_domain || "localhost"}`, models: (body.models || []).join("、") || t("examplesNone") }));
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function copy(label: string, text?: string) {
    if (!text) {
      setMessage(t("examplesNeedRefresh"));
      return;
    }
    await navigator.clipboard.writeText(text);
    setMessage(t("examplesCopied", { label }));
  }

  const snippets: Snippet[] = focusCurl
    ? [{ id: "curl", copyLabel: "curl", copyButton: t("copyCurl"), text: focusCurl }]
    : [
        { id: "curl", copyLabel: "curl", copyButton: t("copyCurl"), text: docs.examples?.curl },
        { id: "python", copyLabel: "Python", copyButton: t("copyPython"), text: docs.examples?.python },
        { id: "node", copyLabel: "Node.js", copyButton: t("copyNode"), text: docs.examples?.node },
      ];
  const active = snippets.find((item) => item.id === tab) ?? snippets[0];
  const sample = active?.text;

  if (!catalogOk) {
    return (
      <div data-testid="model-entry-error" data-reason="catalog" role="alert">
        <EmptyLedger title={t("examplesFail")} detail={catalogMessage || t("examplesHint")} />
        <Button asChild variant="outline" size="sm" className="mt-3">
          <Link href={catalogHref}>{t("pgBackCatalog")}</Link>
        </Button>
      </div>
    );
  }

  if (modelError === "missing") {
    return (
      <div data-testid="model-entry-error" data-reason="missing" role="alert">
        <EmptyLedger title={t("pgModelMissing")} detail={t("pgModelMissingDetail")} />
        <Button asChild variant="outline" size="sm" className="mt-3">
          <Link href={catalogHref}>{t("pgBackCatalog")}</Link>
        </Button>
      </div>
    );
  }

  if (modelError === "unavailable") {
    return (
      <div data-testid="model-entry-error" data-reason="unavailable" role="alert">
        <EmptyLedger title={t("pgModelUnavailable")} detail={t("pgModelUnavailableDetail", { id: requestedModel || "" })} />
        <Button asChild variant="outline" size="sm" className="mt-3">
          <Link href={catalogHref}>{t("pgBackCatalog")}</Link>
        </Button>
      </div>
    );
  }

  return (
    <Card>
      <p className="mb-3 text-sm text-ink-secondary">
        {requestedModel ? t("examplesNoInteractive", { id: requestedModel, path: focusPath || "/v1" }) : t("examplesLead")}
      </p>
      <ActionRow className="mb-3 gap-3">
        <Button variant="outline" onClick={() => void refresh()}>
          {t("refreshExamples")}
        </Button>
        {snippets.map((item) => (
          <Button key={item.id} variant="outline" onClick={() => void copy(item.copyLabel, item.text)}>
            {item.copyButton}
          </Button>
        ))}
      </ActionRow>
      <div className="mb-3 flex gap-1" role="tablist" aria-label={t("examplesTitle")}>
        {snippets.map((item) => (
          <button
            key={item.id}
            type="button"
            role="tab"
            aria-selected={active?.id === item.id}
            onClick={() => setTab(item.id)}
            className={`rounded-control px-3 py-1.5 text-sm ${
              active?.id === item.id ? "bg-brand-soft text-brand-emphasis" : "border border-hairline text-ink-mute"
            }`}
          >
            {item.id === "curl" ? t("examplesTabCurl") : item.id === "python" ? t("examplesTabPython") : t("examplesTabNode")}
          </button>
        ))}
      </div>
      <pre
        data-testid="example-sample"
        data-example-lang={active?.id || "curl"}
        data-example-model={requestedModel || ""}
        className="overflow-x-auto rounded-card border border-hairline bg-ink p-4 font-mono text-xs text-canvas"
      >
        {sample || t("clickRefresh")}
      </pre>
      <p className="mt-3 text-sm text-ink-secondary">{docs.notes?.auth}</p>
      <p className="mt-3 text-sm text-ink-secondary">{docs.notes?.errors}</p>
      <p className="mt-1 text-sm text-ink-secondary">{docs.notes?.rate_limit}</p>
      <p className="mt-1 text-sm text-ink-secondary">{docs.notes?.webhook}</p>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
