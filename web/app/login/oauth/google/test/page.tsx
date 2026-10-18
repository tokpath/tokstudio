"use client";
import { Suspense, useEffect, useState } from "react";
import { useSearchParams } from "next/navigation";
import { z } from "zod";
import { newActionKey, request } from "@/lib/api-client";
import { useI18n } from "@/components/i18n";
import { ErrorPanel, Heading } from "@/components/product/shared";
import { Card } from "@/components/ui/card";

function TestStart() {
  const { t } = useI18n();
  const search = useSearchParams();
  const [error, setError] = useState<unknown>();
  useEffect(() => {
    const ticket = search.get("ticket") || "";
    if (!ticket) {
      setError(new Error("测试链接缺少凭据。请从品牌域名配置重新生成。"));
      return;
    }
    let cancelled = false;
    void request("/api/v1/auth/google/test", z.object({ url: z.string() }), { method: "POST", actionKey: newActionKey(), body: { ticket } }).then(result => {
      if (!cancelled) window.location.assign(result.url);
    }).catch(reason => { if (!cancelled) setError(reason); });
    return () => { cancelled = true; };
  }, [search]);
  return <main className="mx-auto max-w-lg px-5 py-14"><Heading title={t("测试 Google 回调")} description={t("即将前往 Google。测试不会创建客户或登录。")} /><Card>{error ? <ErrorPanel error={error} /> : <p>{t("正在前往 Google…")}</p>}</Card></main>;
}

export default function Page() {
  return <Suspense fallback={<p className="p-8">正在加载…</p>}><TestStart /></Suspense>;
}
