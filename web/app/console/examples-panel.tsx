"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { apiBase } from "@/lib/api";

type DocsContext = {
  brand?: { name?: string; api_domain?: string };
  models?: string[];
  examples?: { curl?: string; python?: string; node?: string; messages?: string; video?: string };
  notes?: { auth?: string; errors?: string; rate_limit?: string; webhook?: string };
};

type ExampleTab = "curl" | "python" | "node";

export default function ExamplesPanel() {
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
    setMessage(t("examplesMeta", { host: body.brand?.api_domain || "localhost", models: (body.models || []).join("、") || t("examplesNone") }));
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

  const sample =
    tab === "python" ? docs.examples?.python : tab === "node" ? docs.examples?.node : docs.examples?.curl;

  return (
    <Card>
      <p className="mb-3 text-sm text-ink-secondary">{t("examplesLead")}</p>
      <div className="mb-3 flex flex-wrap gap-3">
        <Button variant="outline" onClick={() => void refresh()}>
          {t("refreshExamples")}
        </Button>
        <Button variant="outline" onClick={() => void copy("curl", docs.examples?.curl)}>
          {t("copyCurl")}
        </Button>
        <Button variant="outline" onClick={() => void copy("Python", docs.examples?.python)}>
          {t("copyPython")}
        </Button>
        <Button variant="outline" onClick={() => void copy("Node.js", docs.examples?.node)}>
          {t("copyNode")}
        </Button>
      </div>
      <div className="mb-3 flex gap-1" role="tablist" aria-label={t("examplesTitle")}>
        {(
          [
            ["curl", t("examplesTabCurl")],
            ["python", t("examplesTabPython")],
            ["node", t("examplesTabNode")],
          ] as const
        ).map(([id, label]) => (
          <button
            key={id}
            type="button"
            role="tab"
            aria-selected={tab === id}
            onClick={() => setTab(id)}
            className={`rounded-control px-3 py-1.5 text-sm ${
              tab === id ? "bg-brand-soft text-brand-emphasis" : "border border-hairline text-ink-mute"
            }`}
          >
            {label}
          </button>
        ))}
      </div>
      <pre className="overflow-x-auto rounded-card border border-hairline bg-ink p-4 font-mono text-xs text-canvas">
        {sample || t("clickRefresh")}
      </pre>
      <p className="mt-3 text-sm text-ink-secondary">{docs.notes?.errors}</p>
      <p className="mt-1 text-sm text-ink-secondary">{docs.notes?.rate_limit}</p>
      <p className="mt-1 text-sm text-ink-secondary">{docs.notes?.webhook}</p>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
