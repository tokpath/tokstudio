"use client";

import { useEffect, useRef, useState, type FormEvent } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { AuthCard } from "@/components/auth-card";
import { EmailVerificationStatus, useEmailCooldown } from "@/components/email-verification-status";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { apiBase } from "@/lib/api";
import { emailAction, emailErrorKey, emailTokenFromFragment } from "@/lib/email-auth";
import { safeNextPath } from "@/lib/login-next";

export function EmailActionForm({ kind }: { kind: "forgot" | "reset" | "verify" }) {
  const t = useTranslations("emailAuth");
  const search = useSearchParams();
  const [token, setToken] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [user, setUser] = useState<{ email: string; email_verified: boolean } | null>(null);
  const [done, setDone] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [requested, setRequested] = useState(false);
  const cooldown = useEmailCooldown();
  const initialFragment = useRef<string | null>(null);
  const next = safeNextPath(search.get("next")) || "/enter";

  useEffect(() => {
    let cancelled = false;
    if (initialFragment.current === null) initialFragment.current = window.location.hash;
    const fragment = initialFragment.current;
    setToken(emailTokenFromFragment(fragment));
    if (kind === "verify" && !fragment) {
      void fetch(`${apiBase}/v1/me`, { credentials: "include" }).then(response => response.ok ? response.json() : null).then(body => { if (!cancelled) { setUser(body?.user || null); setLoaded(true); } }).catch(() => { if (!cancelled) setLoaded(true); });
    } else { setLoaded(true); }
    return () => { cancelled = true; };
  }, [kind]);

  async function submit(event: FormEvent) {
    event.preventDefault(); setError("");
    if (kind === "reset") {
      if (password !== confirmation) { setError(t("passwordMismatch")); return; }
      if (password.length < 8 || new TextEncoder().encode(password).length > 72) { setError(t("passwordRule")); return; }
    }
    setBusy(true);
    try {
      if (kind === "forgot" || (kind === "verify" && !token)) {
        const body = await emailAction(kind === "forgot" ? "/v1/auth/password/forgot" : "/v1/auth/email/verification/request", { email });
        setRequested(true); cooldown.start(body.retry_after || 60);
      } else {
        await emailAction(kind === "reset" ? "/v1/auth/password/reset" : "/v1/auth/email/verify", { token, ...(kind === "reset" ? { new_password: password } : {}) });
        window.history.replaceState(null, "", window.location.pathname + window.location.search);
        setToken(""); setPassword(""); setConfirmation(""); setDone(true);
      }
    } catch (error) { setError(t(emailErrorKey(error))); }
    finally { setBusy(false); }
  }

  const requesting = kind === "forgot" || (kind === "verify" && !token);
  return <AuthCard title={t(kind === "forgot" ? "forgotTitle" : kind === "reset" ? "resetTitle" : "verifyTitle")}>
    {!loaded ? <p role="status">{t("loading")}</p> : done ? <div className="space-y-5">
      <p role="status" className="text-sm leading-relaxed">{t(kind === "reset" ? "resetSuccess" : "verifySuccess")}</p>
      <Link className="inline-flex text-brand-emphasis hover:underline" href={kind === "reset" ? "/login" : next}>{t(kind === "reset" ? "backToLogin" : "continue")}</Link>
    </div> : kind === "reset" && !token ? <div className="space-y-5"><p role="alert" className="text-sm text-danger">{t("invalidLink")}</p><Link href="/forgot-password" className="text-brand-emphasis hover:underline">{t("requestAgain")}</Link></div> : kind === "verify" && !token && user ? <div className="space-y-6"><EmailVerificationStatus email={user.email} verified={user.email_verified === true} initiallyQueued={search.get("sent") === "1"} /><Link className="inline-block text-brand-emphasis hover:underline" href={next}>{t("continue")}</Link></div> : <>
      <p className="mb-5 text-sm leading-relaxed text-ink-secondary">{t(kind === "forgot" ? "forgotHint" : kind === "reset" ? "resetHint" : token ? "confirmVerificationHint" : "requestVerificationHint")}</p>
      <form onSubmit={submit} className="space-y-4">
        {requesting && <label className="grid gap-2 text-sm" htmlFor="recovery-email">{t("email")}<Input id="recovery-email" type="email" autoComplete="email" required value={email} disabled={busy} onChange={event => setEmail(event.target.value)} /></label>}
        {kind === "reset" && <>
          <label className="grid gap-2 text-sm" htmlFor="new-password">{t("newPassword")}<Input id="new-password" type="password" autoComplete="new-password" minLength={8} required disabled={busy} value={password} onChange={event => { setPassword(event.target.value); setError(""); }} /></label>
          <label className="grid gap-2 text-sm" htmlFor="confirm-password">{t("confirmPassword")}<Input id="confirm-password" type="password" autoComplete="new-password" minLength={8} required disabled={busy} value={confirmation} onChange={event => { setConfirmation(event.target.value); setError(""); }} /></label>
          <p className="text-xs text-ink-secondary">{t("passwordRule")}</p>
        </>}
        <Button className="w-full" type="submit" disabled={busy || (requesting && cooldown.remaining > 0)}>{busy ? t("working") : requesting && cooldown.remaining ? t("retryIn", { seconds: cooldown.remaining }) : t(kind === "reset" ? "resetSubmit" : kind === "verify" && token ? "verifySubmit" : "sendEmail")}</Button>
      </form>
      {requested && <p role="status" className="mt-4 text-sm leading-relaxed">{t(kind === "forgot" ? "resetRequested" : "verificationRequested")}</p>}
      {error && <p role="alert" className="mt-4 text-sm text-danger">{error} {kind === "reset" && <Link href="/forgot-password" className="underline">{t("requestAgain")}</Link>}{kind === "verify" && token && <button type="button" className="underline" onClick={() => { window.history.replaceState(null, "", window.location.pathname + window.location.search); setToken(""); setError(""); }}>{t("requestAgain")}</button>}</p>}
      <Link href="/login" className="mt-6 inline-block text-sm text-brand-emphasis hover:underline">{t("backToLogin")}</Link>
    </>}
  </AuthCard>;
}
