"use client";

import { useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslations } from "next-intl";
import { z } from "zod";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Form } from "@/components/ui/form";
import { apiBase } from "@/lib/api";
import { resolveLocale } from "@/lib/i18n";

type MeUser = {
  email?: string;
  display_name?: string;
  locale?: string;
  channel_org_id?: string;
};

export default function SettingsPanel() {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const chrome = useTranslations("chrome");
  const [user, setUser] = useState<MeUser | null>(null);
  const [message, setMessage] = useState(t("settingsHint"));
  const profileSchema = useMemo(
    () =>
      z.object({
        display_name: z.string().trim().min(1, t("displayRequired")),
        locale: z.enum(["zh", "en", "ja"]),
      }),
    [t],
  );
  const passwordSchema = useMemo(
    () =>
      z.object({
        current_password: z.string().min(1, t("curPassRequired")),
        new_password: z.string().min(8, t("newPassRequired")),
      }),
    [t],
  );
  const profileForm = useForm<z.infer<typeof profileSchema>>({
    resolver: zodResolver(profileSchema),
    defaultValues: { display_name: "", locale: "zh" },
  });
  const passwordForm = useForm<z.infer<typeof passwordSchema>>({
    resolver: zodResolver(passwordSchema),
    defaultValues: { current_password: "", new_password: "" },
  });

  async function refresh() {
    const response = await fetch(`${apiBase}/v1/me`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || tc("notLoggedIn"));
      return;
    }
    const next = (body.user || {}) as MeUser;
    setUser(next);
    profileForm.reset({ display_name: next.display_name || "", locale: resolveLocale(next.locale) as "zh" | "en" | "ja" });
    setMessage(t("profileRefreshed"));
  }

  return (
    <Card>
      <CardTitle className="mb-4 text-lg font-semibold tracking-tight">{t("settingsTitle")}</CardTitle>
      <p className="mb-4 text-sm text-ink-secondary">
        {t("settingsMeta", { email: user?.email ?? "—", channel: user?.channel_org_id ?? "—" })}
      </p>
      <Form {...profileForm}>
        <form
          className="mb-4 flex flex-wrap items-end gap-3"
          onSubmit={profileForm.handleSubmit(async (values) => {
            const response = await fetch(`${apiBase}/v1/me`, {
              method: "PATCH",
              credentials: "include",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify({ display_name: values.display_name, locale: values.locale }),
            });
            const body = await response.json();
            if (!response.ok) {
              setMessage(body.error?.message || t("saveFail"));
              return;
            }
            const next = (body.user || {}) as MeUser;
            setUser(next);
            document.cookie = `NEXT_LOCALE=${resolveLocale(next.locale)}; path=/; max-age=31536000`;
            setMessage(t("profileSaved"));
          })}
        >
          <TextField control={profileForm.control} name="display_name" label={t("displayName")} showLabel={false} />
          <label className="flex flex-col gap-1 text-xs text-ink-mute">
            {t("uiLang")}
            <select
              aria-label={t("uiLang")}
              className="h-10 rounded-control border border-hairline bg-canvas px-3 text-sm text-ink"
              {...profileForm.register("locale")}
            >
              <option value="zh">{chrome("localeZh")}</option>
              <option value="en">{chrome("localeEn")}</option>
              <option value="ja">{chrome("localeJa")}</option>
            </select>
          </label>
          <Button type="button" variant="outline" onClick={refresh}>
            {t("refreshProfile")}
          </Button>
          <Button type="submit">{t("saveProfile")}</Button>
        </form>
      </Form>
      <Form {...passwordForm}>
        <form
          className="flex flex-wrap items-end gap-3"
          onSubmit={passwordForm.handleSubmit(async (values) => {
            const response = await fetch(`${apiBase}/v1/me/password`, {
              method: "POST",
              credentials: "include",
              headers: { "Content-Type": "application/json" },
              body: JSON.stringify(values),
            });
            const body = await response.json();
            setMessage(response.ok ? t("passUpdated") : body.error?.message || t("passFail"));
            if (response.ok) {
              passwordForm.reset();
            }
          })}
        >
          <TextField control={passwordForm.control} name="current_password" label={t("curPass")} type="password" showLabel={false} />
          <TextField control={passwordForm.control} name="new_password" label={t("newPass")} placeholder={t("newPassPh")} type="password" showLabel={false} />
          <Button type="submit" variant="outline">
            {t("changePass")}
          </Button>
        </form>
      </Form>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
