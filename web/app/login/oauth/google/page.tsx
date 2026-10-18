"use client";
import { Suspense, useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { z } from "zod";
import { SessionSchema } from "@/lib/contracts";
import { safeCustomerNext } from "@/lib/customer-next";
import { newActionKey, request, setCSRFToken } from "@/lib/api-client";
import { useI18n } from "@/components/i18n";
import { ErrorPanel, Heading, Notice } from "@/components/product/shared";
import { Card } from "@/components/ui/card";

const FinishSchema = z.union([SessionSchema, z.object({ kind: z.literal("google_test"), message: z.string().optional() })]);

function Callback() {
  const { t } = useI18n();
  const router = useRouter(), search = useSearchParams(), queryClient = useQueryClient();
  const [error, setError] = useState<unknown>();
  const [tested, setTested] = useState("");
  useEffect(() => {
    const code = search.get("code") || "", state = search.get("state") || "";
    if (!code || !state) {
      setError(new Error(search.get("error") || "Google 登录没有返回授权码，请从登录页重新开始。"));
      return;
    }
    let cancelled = false;
    void request("/api/v1/auth/google/finish", FinishSchema, { method: "POST", actionKey: newActionKey(), body: { code, state } }).then(result => {
      if (cancelled) return;
      if (result.kind === "google_test") {
        setTested(result.message || "这个域名的 Google 回调测试已通过。回到品牌配置并启用后，客户即可使用。");
        return;
      }
      if (result.kind === "customer") {
        queryClient.setQueryData(["session"], result);
        setCSRFToken(result.csrf_token);
        router.replace(safeCustomerNext(search.get("next")) || "/workspace");
      }
    }).catch(reason => { if (!cancelled) setError(reason); });
    return () => { cancelled = true; };
  }, [queryClient, router, search]);
  return <main className="mx-auto max-w-lg px-5 py-14"><Heading title={t("Google 登录")} description={t("正在核对当前品牌的 Google 账号。")} /><Card>{tested ? <Notice>{t(tested)}</Notice> : error ? <ErrorPanel error={error} /> : <p>{t("正在完成登录…")}</p>}</Card></main>;
}

export default function Page() {
  return <Suspense fallback={<p className="p-8">正在加载…</p>}><Callback /></Suspense>;
}
