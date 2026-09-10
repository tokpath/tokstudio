"use client";

import { Suspense, useEffect, useRef } from "react";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { apiBase } from "@/lib/api";
import { CONSOLE_ENTRY_PATH, resolveConsoleHref } from "@/lib/console-home";
import { safeNextPath } from "@/lib/login-next";
import { oauthFailureHref, readStoredNext, sanitizeOAuthError } from "@/lib/google-oauth";

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
      window.location.replace(oauthFailureHref(t("googleFail"), search.get("error_code") || googleError || "authentication_error"));
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
        const body = (await response.json()) as { error?: { message?: string; code?: string } };
        if (!response.ok) {
          window.location.replace(
            oauthFailureHref(sanitizeOAuthError(body.error?.message, t("googleFail")), body.error?.code),
          );
          return;
        }
        const next = safeNextPath(readStoredNext(typeof sessionStorage === "undefined" ? null : sessionStorage));
        if (next && next !== CONSOLE_ENTRY_PATH) {
          window.location.replace(next);
          return;
        }
        window.location.replace(await resolveConsoleHref());
      } catch {
        window.location.replace(oauthFailureHref(t("googleFail")));
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
    <Suspense fallback={<main className="flex min-h-svh items-center justify-center px-6 text-ink-secondary">{t("googleFinishing")}</main>}>
      <GoogleOAuthCallback />
    </Suspense>
  );
}
