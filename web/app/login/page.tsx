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
import { safeNextPath } from "@/lib/login-next";
import { KeyRound, LogIn, Mail, Shield, UserPlus } from "lucide-react";
import { GitHubMark, GoogleMark } from "@/components/oauth-marks";
import { useTranslations } from "next-intl";

function LoginForm() {
  const t = useTranslations("login");
  const th = useTranslations("home");
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
    <main className="mx-auto grid min-h-[calc(100vh-8rem)] w-full max-w-[1120px] items-center gap-12 px-6 py-16 lg:grid-cols-[1fr_26rem] lg:gap-20 lg:py-20">
      <section className="hidden max-w-xl lg:block">
        <p className="th-eyebrow text-brand-emphasis">{t("eyebrow")}</p>
        <p className="th-display mt-5">
          {th("h1a")}
          <span className="text-brand-emphasis">{th("h1b")}</span>
          {th("h1c")}
        </p>
        <p className="mt-6 text-base leading-relaxed text-ink-secondary">{th("lead")}</p>
      </section>
      <section className="w-full rounded-card border border-hairline bg-canvas-raised p-8">
        <p className="th-eyebrow text-brand-emphasis lg:hidden">{t("eyebrow")}</p>
        <h1 className="mt-3 text-[32px] font-semibold leading-tight tracking-tight sm:text-[40px]">{t("title")}</h1>

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
          <div className="mt-3 flex flex-col gap-2 rounded-card border border-hairline bg-canvas p-3">
            <label className="text-[13px] text-ink-secondary" htmlFor="google-email">
              {t("googleEmail")}
            </label>
            <input
              id="google-email"
              className="rounded-control border border-hairline bg-canvas-raised px-3 py-2 text-sm"
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
              <div className="flex flex-wrap items-end gap-2">
                <TextField control={form.control} name="otp" label={t("otp")} placeholder={t("otpPlaceholder")} className="flex-1" icon={Shield} />
                <Button type="button" variant="ghost" onClick={requestOtp}>
                  {t("sendOtp")}
                </Button>
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
              <>
                <Button type="button" variant="outline" className="w-full" onClick={loginOtp}>
                  {t("otpLogin")}
                </Button>
                <p className="text-[13px] leading-relaxed text-ink-mute">{t("forgot")}</p>
              </>
            ) : null}
            <p className="text-sm text-hold">{message}</p>
            <p className="text-sm text-ink-secondary">
              {mode === "login" ? (
                <>
                  {t("noAccount")}{" "}
                  <button type="button" className="text-brand-emphasis" onClick={() => setMode("register")}>
                    {t("goRegister")}
                  </button>
                </>
              ) : (
                <>
                  {t("hasAccount")}{" "}
                  <button type="button" className="text-brand-emphasis" onClick={() => setMode("login")}>
                    {t("goLogin")}
                  </button>
                </>
              )}
            </p>
            <p className="text-[13px] leading-relaxed text-ink-mute">
              {t("terms")} <Link href="/terms">{t("termsLink")}</Link> {t("and")} <Link href="/privacy">{t("privacy")}</Link>. {t("backHome")}{" "}
              <Link href="/">{t("public")}</Link>.
            </p>
          </form>
        </Form>
      </section>
    </main>
  );
}

export default function LoginPage() {
  const t = useTranslations("login");
  return (
    <Suspense fallback={<main className="mx-auto max-w-md px-6 py-20 text-ink-secondary">{t("fallback")}</main>}>
      <LoginForm />
    </Suspense>
  );
}
