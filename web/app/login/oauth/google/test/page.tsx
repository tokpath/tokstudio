"use client";

import { Suspense, useEffect, useRef, useState } from "react";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { apiClient } from "@/lib/client";

import { GOOGLE_TEST_STATE } from "@/lib/google-oauth";

function GoogleTest() {
  const search = useSearchParams();
  const t = useTranslations("oemGoogle");
  const ran = useRef(false);
  const [failed, setFailed] = useState(Boolean(search.get("error")));
  const success = search.get("result") === "success";
  useEffect(() => {
    if (ran.current) return;
    ran.current = true;
    if (success || search.get("error")) { sessionStorage.removeItem(GOOGLE_TEST_STATE); return; }
    const ticket = search.get("ticket");
    if (!ticket) { setFailed(true); return; }
    // Remove the one-time test ticket from browser history and referrers.
    window.history.replaceState(null, "", "/login/oauth/google/test");
    void apiClient<{ state: string; auth_url: string }>("GET", `/v1/auth/google/test/start?ticket=${encodeURIComponent(ticket)}`)
      .then(body => { sessionStorage.setItem(GOOGLE_TEST_STATE, body.state); window.location.replace(body.auth_url); })
      .catch(() => setFailed(true));
  }, [search, success]);
  return <main className="flex min-h-svh items-center justify-center px-6 py-12">
    <section className="max-w-lg rounded-card border border-hairline bg-canvas-raised p-6">
      <h1 className="text-xl font-semibold">{t("testTitle")}</h1>
      <p className={`mt-3 text-sm ${failed ? "text-danger" : "text-ink-secondary"}`} role={failed ? "alert" : "status"}>{success ? t("testSuccess") : failed ? t("testFailure") : t("testRedirecting")}</p>
    </section>
  </main>;
}
export default function GoogleTestPage() {
  return <Suspense><GoogleTest /></Suspense>;
}
