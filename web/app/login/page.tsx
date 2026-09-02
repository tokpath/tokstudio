"use client";

import { Suspense, useMemo, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import { apiBase } from "@/lib/api";
import { useBrand } from "@/components/brand-context";
import { BrandLogo } from "@/components/brand-logo";
import { LocaleSwitch } from "@/components/locale-switch";
import { ThemeToggle } from "@/components/theme-toggle";
import { safeNextPath } from "@/lib/login-next";
import { KeyRound, LogIn, Mail, Shield, UserPlus } from "lucide-react";
import { GitHubMark, GoogleMark } from "@/components/oauth-marks";
import { useTranslations } from "next-intl";

function LoginForm() {
  const t = useTranslations("login");
  const brand = useBrand();
  const siteName = brand?.name || "TokenHub";
  const schema = useMemo(
    () =>
      z.object({
        email: z.string().trim().email(t("emailInvalid")),
        password: z.string().min(8, t("passMin")),
        promo: z.string().trim(),
        otp: z.string().trim(),
      }),
    [t],
  );
  const search = useSearchParams();
  const [message, setMessage] = useState("");
  const [mode, setMode] = useState<"login" | "register">("login");
  const [googleState, setGoogleState] = useState("");
  const [googleEmail, setGoogleEmail] = useState("");
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { email: "", password: "", promo: "", otp: "" },
  });

  function goNext() {
    const next = safeNextPath(search.get("next"));
    window.location.href = next || "/app";
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
      setMessage(t("registered", { channel: body.session?.user?.channel_org_id || "—" }));
      goNext();
      return;
    }
    setMessage(body.error?.message || t("fail"));
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
      setMessage(t("welcome", { email: body.session?.user?.email || "" }));
      goNext();
      return;
    }
    setMessage(body.error?.message || t("fail"));
  }

  async function googleStart() {
    const response = await fetch(`${apiBase}/v1/auth/google/start`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || t("googleUnavailable"));
      return;
    }
    if (body.mock) {
      setGoogleState(body.state || "");
      setGoogleEmail(form.getValues("email"));
      setMessage(t("googleDevHint"));
      return;
    }
    if (body.auth_url) {
      window.location.href = body.auth_url;
      return;
    }
    setMessage(t("googleNoRedirect"));
  }

  async function googleFinish() {
    const email = googleEmail.trim();
    if (!googleState || !email.includes("@")) {
      setMessage(t("googleNeedEmail"));
      return;
    }
    const response = await fetch(`${apiBase}/v1/auth/google/callback`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ state: googleState, code: `mock:${email}` }),
    });
    const body = await response.json();
    if (response.ok) {
      setMessage(t("welcome", { email: body.session?.user?.email || "" }));
      goNext();
      return;
    }
    setMessage(body.error?.message || t("googleFail"));
  }

  function githubStart() {
    setMessage(t("githubMissing"));
  }

  async function requestOtp() {
    const email = form.getValues("email");
    const response = await fetch(`${apiBase}/v1/auth/otp/request`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, purpose: "login" }),
    });
    const body = await response.json();
    setMessage(
      response.ok
        ? body.dev_code
          ? t("otpSentDev", { code: body.dev_code })
          : t("otpSent")
        : body.error?.message || t("otpSendFail"),
    );
  }

  async function loginOtp() {
    const values = form.getValues();
    const response = await fetch(`${apiBase}/v1/auth/otp/verify`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email: values.email, code: values.otp }),
    });
    const body = await response.json();
    if (response.ok) {
      setMessage(t("welcome", { email: body.session?.user?.email || "" }));
      goNext();
      return;
    }
    setMessage(body.error?.message || t("otpInvalid"));
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

        <div className="mt-8 flex flex-col gap-3">
          <Button type="button" variant="outline" className="w-full" onClick={githubStart}>
            <GitHubMark />
            {t("github")}
          </Button>
          <Button type="button" variant="outline" className="w-full" onClick={googleStart}>
            <GoogleMark />
            {t("google")}
          </Button>
        </div>
        {googleState ? (
          <div className="mt-4 flex flex-col gap-3 rounded-card border border-hairline bg-canvas p-4">
            <label className="text-[13px] text-ink-secondary" htmlFor="google-email">
              {t("googleEmail")}
            </label>
            <input
              id="google-email"
              className="h-10 w-full rounded-control border border-hairline bg-canvas-raised px-3 text-sm"
              value={googleEmail}
              onChange={(event) => setGoogleEmail(event.target.value)}
              placeholder="you@gmail.com"
              type="email"
            />
            <Button type="button" variant="outline" onClick={googleFinish}>
              <GoogleMark />
              {t("continueGoogle")}
            </Button>
          </div>
        ) : null}

        <div className="my-7 flex items-center gap-3 text-[13px] text-ink-mute">
          <span className="h-px flex-1 bg-hairline" />
          {t("or")}
          <span className="h-px flex-1 bg-hairline" />
        </div>

        <Form {...form}>
          <form className="flex flex-col gap-4" onSubmit={(event) => event.preventDefault()}>
            <TextField control={form.control} name="email" label={t("email")} placeholder="m@example.com" icon={Mail} />
            <TextField control={form.control} name="password" label={t("password")} placeholder={t("passwordPh")} type="password" icon={KeyRound} />
            {mode === "login" ? (
              <div>
                <TextField control={form.control} name="otp" label={t("otp")} placeholder={t("otpPlaceholder")} icon={Shield} />
                <button
                  type="button"
                  className="mt-2 text-[13px] text-brand-emphasis hover:underline"
                  onClick={requestOtp}
                >
                  {t("sendOtp")}
                </button>
              </div>
            ) : null}
            {mode === "register" ? (
              <TextField control={form.control} name="promo" label={t("promo")} placeholder={t("promoPh")} />
            ) : null}
            <Button
              type="button"
              className="mt-1 w-full"
              onClick={form.handleSubmit(mode === "login" ? login : register)}
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
            {mode === "login" ? (
              <Button type="button" variant="outline" className="w-full" onClick={loginOtp}>
                {t("otpLogin")}
              </Button>
            ) : null}
            {message ? <p className="text-sm leading-relaxed text-hold">{message}</p> : null}
            <p className="pt-1 text-sm text-ink-secondary">
              {mode === "login" ? (
                <>
                  {t("noAccount")}{" "}
                  <button type="button" className="text-brand-emphasis hover:underline" onClick={() => setMode("register")}>
                    {t("goRegister")}
                  </button>
                </>
              ) : (
                <>
                  {t("hasAccount")}{" "}
                  <button type="button" className="text-brand-emphasis hover:underline" onClick={() => setMode("login")}>
                    {t("goLogin")}
                  </button>
                </>
              )}
            </p>
          </form>
        </Form>
        <div className="mt-8 border-t border-hairline pt-5">
          {mode === "login" ? <p className="text-[13px] leading-relaxed text-ink-mute">{t("forgot")}</p> : null}
          <p className={`text-[13px] leading-relaxed text-ink-mute ${mode === "login" ? "mt-3" : ""}`}>
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
