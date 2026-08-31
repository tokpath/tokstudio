"use client";

import { useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
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

const profileSchema = z.object({
  display_name: z.string().trim().min(1, "请填写显示名"),
  locale: z.enum(["zh", "en", "ja"]),
});

const passwordSchema = z.object({
  current_password: z.string().min(1, "请填写当前密码"),
  new_password: z.string().min(8, "新密码至少 8 位"),
});

export default function SettingsPanel() {
  const [user, setUser] = useState<MeUser | null>(null);
  const [message, setMessage] = useState("登录后可改显示名、界面语言和登录密码。渠道归属不能自己改。");
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
      setMessage(body.error?.message || "未登录");
      return;
    }
    const next = (body.user || {}) as MeUser;
    setUser(next);
    profileForm.reset({ display_name: next.display_name || "", locale: resolveLocale(next.locale) as "zh" | "en" | "ja" });
    setMessage("资料已刷新");
  }

  return (
    <Card className="rounded-card border border-hairline bg-canvas-raised  p-6">
      <CardTitle className="mb-3 text-xl font-medium">个人设置</CardTitle>
      <p className="mb-4 text-sm text-ink-secondary">
        邮箱 {user?.email ?? "—"}，渠道 {user?.channel_org_id ?? "—" }。页面上没有切换渠道的入口。
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
              setMessage(body.error?.message || "保存失败");
              return;
            }
            const next = (body.user || {}) as MeUser;
            setUser(next);
            document.cookie = `NEXT_LOCALE=${resolveLocale(next.locale)}; path=/; max-age=31536000`;
            setMessage("个人资料已保存");
          })}
        >
          <TextField control={profileForm.control} name="display_name" label="显示名" showLabel={false} />
          <label className="flex flex-col gap-1 text-xs text-ink-mute">
            界面语言
            <select
              aria-label="界面语言"
              className="h-10 rounded-lg border border-hairline bg-canvas px-3 text-sm text-ink"
              {...profileForm.register("locale")}
            >
              <option value="zh">中文</option>
              <option value="en">English</option>
              <option value="ja">日本語</option>
            </select>
          </label>
          <Button type="button" variant="outline" onClick={refresh}>
            刷新资料
          </Button>
          <Button type="submit">保存资料</Button>
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
            setMessage(response.ok ? "密码已更新，下次请用新密码登录" : body.error?.message || "改密失败");
            if (response.ok) {
              passwordForm.reset();
            }
          })}
        >
          <TextField control={passwordForm.control} name="current_password" label="当前密码" type="password" showLabel={false} />
          <TextField control={passwordForm.control} name="new_password" label="新密码" placeholder="新密码（至少 8 位）" type="password" showLabel={false} />
          <Button type="submit" variant="outline">
            修改密码
          </Button>
        </form>
      </Form>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
    </Card>
  );
}
