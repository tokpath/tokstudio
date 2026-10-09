"use client";

import { Suspense, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { capturePublicInvitation, readPublicInvitation } from "@/lib/public-invitation";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import { apiBase } from "@/lib/api";
import { useViewer } from "@/components/rbac/viewer-context";
import { useBrand } from "@/components/brand-context";
import { BrandLogo } from "@/components/brand-logo";
import { LocaleSwitch } from "@/components/locale-switch";
import { ThemeToggle } from "@/components/theme-toggle";
import { CONSOLE_ENTRY_PATH, resolveConsoleHref } from "@/lib/console-home";
import { authIntent, storeAuthIntent } from "@/lib/auth-intent";
import { safeNextPath } from "@/lib/login-next";
import {
  type GoogleAuthStatus,
  googleButtonState,
  sanitizeOAuthError,
  storeLoginNext,
} from "@/lib/google-oauth";
import { KeyRound, LogIn, Mail, UserPlus } from "lucide-react";
import { GoogleMark } from "@/components/oauth-marks";
import { useTranslations } from "next-intl";

type AuthError = { code?: string; message?: string; retryable?: boolean };

function LoginForm() {
  const t = useTranslations("login");
  const brand = useBrand();
  const viewer = useViewer();
  const siteName = brand?.name || "TokenHub";
  const schema = useMemo(
    () =>
      z.object({
        email: z.string().trim().email(t("emailInvalid")),
        password: z.string().min(8, t("passMin")),
        promo: z.string().trim(),
      }),
    [t],
  );
  const search = useSearchParams();
  const [message, setMessage] = useState("");
  const [errorBanner, setErrorBanner] = useState("");
  const invitation = authIntent(search).promotionCode;
  const [mode, setMode] = useState<"login" | "register">(invitation ? "register" : "login");
  const [googleStatus, setGoogleStatus] = useState<GoogleAuthStatus | null>(null);
  const [googleLoading, setGoogleLoading] = useState(false);
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { email: "", password: "", promo: invitation },
  });
  const googleUI = googleButtonState(googleStatus, googleLoading);

  useEffect(() => {
    let code=invitation;
    try{capturePublicInvitation(sessionStorage,search,brand?.id || "");code=code || readPublicInvitation(sessionStorage,brand?.id || "");}catch{/* private mode */}
    form.setValue("promo", code);
    if (code) setMode("register");
  }, [invitation, search,brand?.id, form]);

  useEffect(() => {
    const fromCallback = sanitizeOAuthError(search.get("oauth_error"), "");
    if (fromCallback) {
      setErrorBanner(fromCallback);
    }
  }, [search]);

  useEffect(() => {
    let cancelled = false;
    fetch(`${apiBase}/v1/auth/google/status`, { credentials: "include" })
      .then(async (response) => {
        const body = (await response.json()) as GoogleAuthStatus;
        if (!cancelled && response.ok) {
          setGoogleStatus(body);
        } else if (!cancelled) {
          setGoogleStatus({ available: false, configured: false, mock: false });
        }
      })
      .catch(() => {
        if (!cancelled) {
          setGoogleStatus({ available: false, configured: false, mock: false });
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  async function goNext() {
    const next = safeNextPath(search.get("next"));
    if (next && next !== CONSOLE_ENTRY_PATH) {
      window.location.href = next;
      return;
    }
    window.location.href = await resolveConsoleHref();
  }

  useEffect(() => {
    if (!viewer.loading && viewer.signedIn) void goNext();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [viewer.loading, viewer.signedIn, search]);

  function showAuthFailure(body: { error?: AuthError }, fallback: string) {
    const text = sanitizeOAuthError(body.error?.message, fallback);
    setErrorBanner(text);
    setMessage("");
  }

  async function register(values: z.infer<typeof schema>) {
    const response = await fetch(`${apiBase}/v1/auth/register`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ email: values.email, password: values.password, promotion_code: values.promo }),
    });
    const body = await response.json();
    if (response.ok) {
      setErrorBanner("");
      setMessage(t("registered", { channel: body.session?.user?.channel_org_id || "—" }));
      await goNext();
      return;
    }
    showAuthFailure(body, t("fail"));
  }

  async function login(values: z.infer<typeof schema>) {
    const response = await fetch(`${apiBase}/v1/auth/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ email: values.email, password: values.password }),
    });
    const body = await response.json();
    if (response.ok) {
      setErrorBanner("");
      setMessage(t("welcome", { email: body.session?.user?.email || "" }));
      await goNext();
      return;
    }
    showAuthFailure(body, t("fail"));
  }

  async function googleStart() {
    if (googleUI.disabled || googleLoading) {
      return;
    }
    setGoogleLoading(true);
    setErrorBanner("");
    const storage = typeof sessionStorage === "undefined" ? null : sessionStorage;
    const next = safeNextPath(search.get("next"));
    const promo = form.getValues("promo");
    storeLoginNext(storage, next);
    storeAuthIntent(storage, { next, promotionCode: promo });
    const params = new URLSearchParams();
    if (promo) params.set("promotion_code", promo);
    if (next) params.set("next", next);
    const query = params.size ? `?${params}` : "";
    try {
      const response = await fetch(`${apiBase}/v1/auth/google/start${query}`, { credentials: "include" });
      const body = await response.json();
      if (!response.ok) {
        showAuthFailure(body, t("googleUnavailable"));
        setGoogleLoading(false);
        return;
      }
      if (body.auth_url) {
        window.location.href = body.auth_url;
        return;
      }
      setErrorBanner(t("googleNoRedirect"));
      setGoogleLoading(false);
    } catch {
      setErrorBanner(t("googleUnavailable"));
      setGoogleLoading(false);
    }
  }

  async function submit(values: z.infer<typeof schema>) {
    setErrorBanner("");
    try { await (mode === "login" ? login(values) : register(values)); }
    catch { setMessage(""); setErrorBanner(t("networkError")); }
  }



  return (
    <main className="flex min-h-svh w-full flex-col items-center justify-center px-6 py-12">
      <div className="w-full max-w-[26rem]">
        <Link href="/" className="mb-8 flex items-center justify-center gap-2.5 text-ink no-underline">
          <BrandLogo brand={brand} />
          <span className="text-xl font-semibold tracking-tight">{siteName}</span>
        </Link>
        <section className="w-full rounded-card border border-hairline bg-canvas-raised p-8 sm:p-10">
          <div className="flex items-start justify-between gap-3">
            <h1 className="text-2xl font-semibold tracking-tight">{t("title")}</h1>
            <div className="flex shrink-0 items-center">
              <LocaleSwitch />
              <ThemeToggle />
            </div>
          </div>

        {errorBanner ? (
          <div
            role="alert"
            className="mt-6 rounded-control border border-danger/40 bg-danger/10 px-3 py-2 text-sm leading-relaxed text-danger"
          >
            {errorBanner}
          </div>
        ) : null}

        {googleStatus?.available && <>
          <div className="mt-8 flex w-full flex-col gap-3">
            <Button type="button" variant="outline" className="w-full" disabled={googleUI.disabled} aria-disabled={googleUI.disabled} onClick={googleStart}><GoogleMark />{t(googleUI.labelKey)}</Button>
          </div>
          <div className="my-7 flex items-center gap-3 text-[13px] text-ink-mute"><span className="h-px flex-1 bg-hairline" />{t("or")}<span className="h-px flex-1 bg-hairline" /></div>
        </>}

        <Form {...form}>
          <form className="mt-7 flex w-full flex-col gap-4" onSubmit={form.handleSubmit(submit)}>
            <TextField control={form.control} name="email" label={t("email")} placeholder="m@example.com" icon={Mail} />
            <TextField control={form.control} name="password" label={t("password")} placeholder={t("passwordPh")} type="password" icon={KeyRound} />
            {mode === "register" ? (
              <TextField control={form.control} name="promo" label={t("promo")} placeholder={t("promoPh")} />
            ) : null}
            <Button
              type="submit"
              className="mt-1 w-full"
              disabled={form.formState.isSubmitting}
            >
              {mode === "login" ? (
                <>
                  <LogIn />
                  {t("submitLogin")}
                </>
              ) : (
                <>
                  <UserPlus />
                  {t("submitRegister")}
                </>
              )}
            </Button>
            {message ? <p className="text-sm leading-relaxed text-ink-secondary">{message}</p> : null}
            <p className="pt-1 text-sm text-ink-secondary">
              {mode === "login" ? (
                <>
                  {t("noAccount")}{" "}
                  <button type="button" disabled={form.formState.isSubmitting} className="text-brand-emphasis hover:underline" onClick={() => setMode("register")}>
                    {t("goRegister")}
                  </button>
                </>
              ) : (
                <>
                  {t("hasAccount")}{" "}
                  <button type="button" disabled={form.formState.isSubmitting} className="text-brand-emphasis hover:underline" onClick={() => setMode("login")}>
                    {t("goLogin")}
                  </button>
                </>
              )}
            </p>
          </form>
        </Form>
        <div className="mt-8 border-t border-hairline pt-5">
          <p className="text-[13px] leading-relaxed text-ink-mute">
            {t("terms")} <Link href="/terms">{t("termsLink")}</Link> {t("and")} <Link href="/privacy">{t("privacy")}</Link>.
          </p>
        </div>
        </section>
      </div>
    </main>
  );
}

export default function LoginPage() {
  const t = useTranslations("login");
  return (
    <Suspense fallback={<main className="flex min-h-svh items-center justify-center px-6 text-ink-secondary">{t("fallback")}</main>}>
      <LoginForm />
    </Suspense>
  );
}
