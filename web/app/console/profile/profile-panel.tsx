"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { apiBase } from "@/lib/api";
import { loginMethodsOf, profileDash } from "@/lib/user-shell";

type MeUser = {
  email?: string;
  display_name?: string;
  login_methods?: string[];
};

export default function ProfilePanel() {
  const t = useTranslations("shell");
  const [user, setUser] = useState<MeUser | null>(null);
  const [ready, setReady] = useState(false);
  const [googleAvailable, setGoogleAvailable] = useState(false);
  const [linking, setLinking] = useState(false);
  const [linkError, setLinkError] = useState("");
  const tg = useTranslations("oemGoogle");

  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const response = await fetch(`${apiBase}/v1/me`, { credentials: "include" });
        if (!response.ok) {
          if (!cancelled) {
            setUser(null);
            setReady(true);
          }
          return;
        }
        const body = (await response.json()) as { user?: MeUser };
        if (!cancelled) {
          setUser(body.user ?? null);
          setReady(true);
        }
      } catch {
        if (!cancelled) {
          setUser(null);
          setReady(true);
        }
      }
    }
    void load();
    void fetch(`${apiBase}/v1/auth/google/status`, { credentials: "include" }).then(response => response.ok ? response.json() : null).then(body => { if (!cancelled) setGoogleAvailable(body?.available === true); }).catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  async function linkGoogle() {
    setLinking(true); setLinkError("");
    try {
      const response = await fetch(`${apiBase}/v1/me/google/start`, { credentials: "include" });
      const body = await response.json();
      if (!response.ok || !body.auth_url) throw new Error(body.error?.message || tg("failed"));
      window.location.assign(body.auth_url);
    } catch (caught) { setLinkError(caught instanceof Error ? caught.message : tg("failed")); setLinking(false); }
  }

  const methods = loginMethodsOf(user?.login_methods);
  const name = ready ? profileDash(user?.display_name) : "—";
  const email = ready ? profileDash(user?.email) : "—";

  return (
    <section className="rounded-card border border-hairline bg-canvas-raised p-6" aria-label={t("profile")}>
      <dl className="grid gap-5 sm:grid-cols-2">
        <div>
          <dt className="th-eyebrow text-ink-mute">{t("displayName")}</dt>
          <dd data-testid="profile-display-name" className="mt-2 text-sm text-ink">
            {name}
          </dd>
        </div>
        <div>
          <dt className="th-eyebrow text-ink-mute">{t("email")}</dt>
          <dd data-testid="profile-email" className="mt-2 text-sm text-ink">
            {email}
          </dd>
        </div>
        <div className="sm:col-span-2">
          <dt className="th-eyebrow text-ink-mute">{t("loginMethod")}</dt>
          <dd data-testid="profile-login-methods" className="mt-2 flex flex-wrap gap-2">
            {methods.length === 0 ? (
              <span className="text-sm text-ink-mute">—</span>
            ) : (
              methods.map((method) => (
                <span
                  key={method}
                  data-method={method}
                  className="inline-flex items-center rounded-control bg-brand-soft px-2.5 py-1 text-[12px] text-brand-emphasis"
                >
                  {method === "google" ? t("loginGoogle") : t("loginPassword")}
                </span>
              ))
            )}
          </dd>
        </div>
      </dl>
      {googleAvailable && user && !methods.includes("google") ? <Button className="mt-5" variant="outline" disabled={linking} onClick={() => void linkGoogle()}>{tg("linkGoogle")}</Button> : null}
      {linkError ? <p role="alert" className="mt-3 text-sm text-danger">{linkError}</p> : null}
    </section>
  );
}
