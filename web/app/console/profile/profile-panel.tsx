"use client";

import { useEffect, useState } from "react";
import { useTranslations } from "next-intl";
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
    return () => {
      cancelled = true;
    };
  }, []);

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
    </section>
  );
}
