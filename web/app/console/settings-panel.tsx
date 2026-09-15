"use client";

import { useEffect, useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslations } from "next-intl";
import { z } from "zod";
import { ActionRow } from "@/components/console/action-row";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Form } from "@/components/ui/form";
import { SubmitStatus } from "@/components/console/submit-status";
import { apiBase } from "@/lib/api";
import { resolveLocale } from "@/lib/i18n";
import { errorMessageFromBody, readResponseBody } from "@/lib/submit-result";

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
  const [passOpen, setPassOpen] = useState(false);
  const [message, setMessage] = useState(t("settingsHint"));
  const [passError, setPassError] = useState("");
  const [savingProfile, setSavingProfile] = useState(false);
  const [changingPass, setChangingPass] = useState(false);
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
    try {
      const response = await fetch(`${apiBase}/v1/me`, { credentials: "include" });
      const body = await readResponseBody(response);
      if (!response.ok) {
        setMessage(errorMessageFromBody(body, tc("notLoggedIn")));
        return;
      }
      const next = ((body as { user?: MeUser }).user || {}) as MeUser;
      setUser(next);
      profileForm.reset({ display_name: next.display_name || "", locale: resolveLocale(next.locale) as "zh" | "en" | "ja" });
      setMessage(t("profileRefreshed"));
    } catch {
      setMessage(tc("listNetwork"));
    }
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function handlePassOpenChange(open: boolean) {
    setPassOpen(open);
    setPassError("");
    if (!open) {
      passwordForm.reset();
    }
  }

  return (
    <Card>
      <p className="mb-4 text-sm text-ink-secondary">
        {t("settingsMeta", { email: user?.email ?? "—", channel: user?.channel_org_id ?? "—" })}
      </p>
      <Form {...profileForm}>
        <form
          className="mb-4 flex flex-wrap items-end gap-3"
          onSubmit={profileForm.handleSubmit(async (values) => {
            setSavingProfile(true);
            try {
              const response = await fetch(`${apiBase}/v1/me`, {
                method: "PATCH",
                credentials: "include",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ display_name: values.display_name, locale: values.locale }),
              });
              const body = await readResponseBody(response);
              if (!response.ok) {
                setMessage(errorMessageFromBody(body, t("saveFail")));
                return;
              }
              const next = ((body as { user?: MeUser }).user || {}) as MeUser;
              setUser(next);
              document.cookie = `NEXT_LOCALE=${resolveLocale(next.locale)}; path=/; max-age=31536000`;
              setMessage(t("profileSaved"));
            } catch {
              setMessage(tc("listNetwork"));
            } finally {
              setSavingProfile(false);
            }
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
          <ActionRow className="gap-3">
            <Button type="button" variant="outline" onClick={() => void refresh()}>
              {t("refreshProfile")}
            </Button>
            <Button type="submit" disabled={savingProfile}>
              {savingProfile ? tc("submitting") : t("saveProfile")}
            </Button>
            <Button type="button" variant="outline" onClick={() => setPassOpen(true)}>
              {t("changePass")}
            </Button>
          </ActionRow>
        </form>
      </Form>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      <Dialog open={passOpen} onOpenChange={handlePassOpenChange}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("changePassTitle")}</DialogTitle>
            <DialogDescription>{t("changePassLead")}</DialogDescription>
          </DialogHeader>
          <Form {...passwordForm}>
            <form
              className="space-y-4"
              onSubmit={passwordForm.handleSubmit(async (values) => {
                setChangingPass(true);
                setPassError("");
                try {
                  const response = await fetch(`${apiBase}/v1/me/password`, {
                    method: "POST",
                    credentials: "include",
                    headers: { "Content-Type": "application/json" },
                    body: JSON.stringify(values),
                  });
                  const body = await readResponseBody(response);
                  if (!response.ok) {
                    setPassError(errorMessageFromBody(body, t("passFail")));
                    return;
                  }
                  passwordForm.reset();
                  setPassOpen(false);
                  setMessage(t("passUpdated"));
                } catch {
                  setPassError(tc("listNetwork"));
                } finally {
                  setChangingPass(false);
                }
              })}
            >
              <TextField control={passwordForm.control} name="current_password" label={t("curPass")} type="password" />
              <TextField
                control={passwordForm.control}
                name="new_password"
                label={t("newPass")}
                placeholder={t("newPassPh")}
                type="password"
              />
              <SubmitStatus error={passError} />
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => handlePassOpenChange(false)}>
                  {tc("cancel")}
                </Button>
                <Button type="submit" disabled={changingPass}>
                  {changingPass ? tc("submitting") : t("changePass")}
                </Button>
              </DialogFooter>
            </form>
          </Form>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
