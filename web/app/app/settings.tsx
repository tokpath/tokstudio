"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";
import { resolveLocale } from "@/lib/i18n";

type MeUser = {
  email?: string;
  display_name?: string;
  locale?: string;
  channel_org_id?: string;
};

export default function SettingsPanel() {
  const [user, setUser] = useState<MeUser | null>(null);
  const [displayName, setDisplayName] = useState("");
  const [locale, setLocale] = useState("zh");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [message, setMessage] = useState("登录后可改显示名、界面语言和登录密码。渠道归属不能自己改。");

  async function refresh() {
    const response = await fetch(`${apiBase}/v1/me`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || "未登录");
      return;
    }
    const next = (body.user || {}) as MeUser;
    setUser(next);
    setDisplayName(next.display_name || "");
    setLocale(resolveLocale(next.locale));
    setMessage("资料已刷新");
  }

  async function saveProfile() {
    const response = await fetch(`${apiBase}/v1/me`, {
      method: "PATCH",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ display_name: displayName, locale }),
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
  }

  async function changePassword() {
    const response = await fetch(`${apiBase}/v1/me/password`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
    });
    const body = await response.json();
    setMessage(response.ok ? "密码已更新，下次请用新密码登录" : body.error?.message || "改密失败");
    if (response.ok) {
      setCurrentPassword("");
      setNewPassword("");
    }
  }

  return (
    <Card className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
      <CardTitle className="mb-3 text-xl font-medium">个人设置</CardTitle>
      <p className="mb-4 text-sm text-slate-400">
        邮箱 {user?.email ?? "—"}，渠道 {user?.channel_org_id ?? "—"}。页面上没有切换渠道的入口。
      </p>
      <div className="mb-4 flex flex-wrap gap-3">
        <Input
          aria-label="显示名"
          placeholder="显示名"
          value={displayName}
          onChange={(e) => setDisplayName(e.target.value)}
        />
        <select
          aria-label="界面语言"
          className="rounded border border-slate-700 bg-slate-950 px-3 py-2"
          value={locale}
          onChange={(e) => setLocale(e.target.value)}
        >
          <option value="zh">中文</option>
          <option value="en">English</option>
          <option value="ja">日本語</option>
        </select>
        <Button variant="outline" onClick={refresh}>
          刷新资料
        </Button>
        <Button onClick={saveProfile}>保存资料</Button>
      </div>
      <div className="flex flex-wrap gap-3">
        <Input
          aria-label="当前密码"
          type="password"
          placeholder="当前密码"
          value={currentPassword}
          onChange={(e) => setCurrentPassword(e.target.value)}
        />
        <Input
          aria-label="新密码"
          type="password"
          placeholder="新密码（至少 8 位）"
          value={newPassword}
          onChange={(e) => setNewPassword(e.target.value)}
        />
        <Button variant="outline" onClick={changePassword}>
          修改密码
        </Button>
      </div>
      <p className="mt-3 text-sm text-slate-300">{message}</p>
    </Card>
  );
}
