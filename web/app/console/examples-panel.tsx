"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { apiBase } from "@/lib/api";

type DocsContext = {
  brand?: { name?: string; api_domain?: string };
  models?: string[];
  examples?: { curl?: string; python?: string; node?: string; messages?: string; video?: string };
  notes?: { auth?: string; errors?: string; rate_limit?: string; webhook?: string };
};

export default function ExamplesPanel() {
  const t = useTranslations("user");
  const [docs, setDocs] = useState<DocsContext>({});
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

  async function copy(label: string, text?: string) {
    if (!text) {
      setMessage(t("examplesNeedRefresh"));
      return;
    }
    await navigator.clipboard.writeText(text);
    setMessage(t("examplesCopied", { label }));
  }

  return (
    <Card>
      <CardTitle className="mb-3 text-xl font-medium">{t("examplesTitle")}</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">{t("examplesLead")}</p>
      <div className="mb-3 flex flex-wrap gap-3">
        <Button variant="outline" onClick={refresh}>
          {t("refreshExamples")}
        </Button>
        <Button variant="outline" onClick={() => copy("curl", docs.examples?.curl)}>
          {t("copyCurl")}
        </Button>
        <Button variant="outline" onClick={() => copy("Python", docs.examples?.python)}>
          {t("copyPython")}
        </Button>
        <Button variant="outline" onClick={() => copy("Node.js", docs.examples?.node)}>
          {t("copyNode")}
        </Button>
      </div>
      <pre className="overflow-x-auto rounded bg-canvas p-3 text-xs text-code-ink">{docs.examples?.curl || t("clickRefresh")}</pre>
      <p className="mt-3 text-sm text-ink-secondary">{docs.notes?.errors}</p>
      <p className="mt-1 text-sm text-ink-secondary">{docs.notes?.rate_limit}</p>
      <p className="mt-1 text-sm text-ink-secondary">{docs.notes?.webhook}</p>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
