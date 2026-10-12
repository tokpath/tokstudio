"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { emailAction, emailErrorKey } from "@/lib/email-auth";
import { apiBase } from "@/lib/api";

export function useEmailCooldown(initialSeconds = 0) {
  const [remaining, setRemaining] = useState(initialSeconds);
  useEffect(() => { if (!remaining) return; const timer = setTimeout(() => setRemaining(value => Math.max(0, value - 1)), 1000); return () => clearTimeout(timer); }, [remaining]);
  return { remaining, start: (seconds: number) => setRemaining(Math.min(900, Math.max(0, seconds))) };
}

export function EmailVerificationStatus({ email, verified, initiallyQueued = false, showEmail = true }: { email: string; verified: boolean; initiallyQueued?: boolean; showEmail?: boolean }) {
  const t = useTranslations("emailAuth");
  const [done, setDone] = useState(verified);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [queued, setQueued] = useState(initiallyQueued);
  const cooldown = useEmailCooldown(initiallyQueued ? 60 : 0);
  useEffect(() => setDone(verified), [verified]);

  async function resend() {
    setBusy(true); setError("");
    try { const body = await emailAction("/v1/me/email/verification"); setQueued(true); cooldown.start(body.retry_after || 60); }
    catch (error) { setError(t(emailErrorKey(error))); }
    finally { setBusy(false); }
  }
  async function refresh() {
    setBusy(true); setError("");
    try { const response = await fetch(`${apiBase}/v1/me`, { credentials: "include" }); if (!response.ok) throw new Error(); const body = await response.json(); setDone(body.user?.email_verified === true); if (!body.user?.email_verified) setError(t("stillUnverified")); }
    catch { setError(t("networkError")); } finally { setBusy(false); }
  }

  return <section className="space-y-3" aria-label={t("verificationStatus")}>
    <p className="text-sm font-medium">{t(done ? "verified" : "unverified")}{showEmail ? ` · ${email}` : ""}</p>
    {!done && <>
      <p className="text-sm leading-relaxed text-ink-secondary">{t(queued ? "verificationQueued" : "verificationHint")}</p>
      <div className="flex flex-wrap gap-3"><Button type="button" variant="outline" disabled={busy || cooldown.remaining > 0} onClick={() => void resend()}>{cooldown.remaining ? t("retryIn", { seconds: cooldown.remaining }) : t("resend")}</Button><Button type="button" variant="ghost" disabled={busy} onClick={() => void refresh()}>{t("refreshStatus")}</Button></div>
    </>}
    {error && <p role="alert" className="text-sm text-danger">{error}</p>}
  </section>;
}
