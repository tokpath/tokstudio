"use client";

import { Suspense, useEffect, useRef } from "react";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { apiBase } from "@/lib/api";
import { CONSOLE_ENTRY_PATH, resolveConsoleHref } from "@/lib/console-home";
import { safeNextPath } from "@/lib/login-next";
import {
  claimOAuthCallback,
  oauthFailureHref,
  readStoredNext,
  releaseOAuthCallback,
  sanitizeOAuthError,
} from "@/lib/google-oauth";

function GoogleOAuthCallback() {
  const t = useTranslations("login");
  const search = useSearchParams();
  const ran = useRef(false);

  useEffect(() => {
    if (ran.current) {
      return;
    }
    ran.current = true;

    const googleError = search.get("error");
    const code = search.get("code") || "";
    const state = search.get("state") || "";
    if (googleError || !code || !state) {
      window.location.replace(
        oauthFailureHref(t("googleFail"), search.get("error_code") || googleError || "authentication_error"),
      );
      return;
    }

    const storage = typeof sessionStorage === "undefined" ? null : sessionStorage;
    if (!claimOAuthCallback(storage, code)) {
      // 同码已在兑换中或刚完成：避免第二次 POST 触发 invalid_grant。
      return;
    }

    async function finish() {
      try {
        const response = await fetch(`${apiBase}/v1/auth/google/callback`, {
          method: "POST",
          credentials: "include",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ state, code }),
        });
        const raw = await response.text();
        let body: { error?: { message?: string; code?: string; param?: string } } = {};
        try {
          body = raw ? (JSON.parse(raw) as typeof body) : {};
        } catch {
          releaseOAuthCallback(storage, code);
          window.location.replace(oauthFailureHref(t("googleFail"), `http_${response.status || 0}`));
          return;
        }
        if (!response.ok) {
          releaseOAuthCallback(storage, code);
          const detail =
            typeof body.error?.param === "string" && body.error.param.length > 0
              ? body.error.param
              : body.error?.code;
          window.location.replace(
            oauthFailureHref(sanitizeOAuthError(body.error?.message, t("googleFail")), detail),
          );
          return;
        }
        const next = safeNextPath(readStoredNext(storage));
        if (next && next !== CONSOLE_ENTRY_PATH) {
          window.location.replace(next);
          return;
        }
        window.location.replace(await resolveConsoleHref());
      } catch {
        releaseOAuthCallback(storage, code);
        window.location.replace(oauthFailureHref(t("googleFail"), "callback_network_error"));
      }
    }

    void finish();
  }, [search, t]);

  return (
    <main className="flex min-h-svh w-full flex-col items-center justify-center px-6 py-12">
      <p className="text-sm text-ink-secondary" role="status">
        {t("googleFinishing")}
      </p>
    </main>
  );
}

export default function GoogleOAuthCallbackPage() {
  const t = useTranslations("login");
  return (
    <Suspense
      fallback={
        <main className="flex min-h-svh items-center justify-center px-6 text-ink-secondary">{t("googleFinishing")}</main>
      }
    >
      <GoogleOAuthCallback />
    </Suspense>
  );
}
