"use client";

import { useState } from "react";
import { apiBase } from "@/lib/api";

export default function LoginPage() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [promo, setPromo] = useState("");
  const [message, setMessage] = useState("");

  async function register() {
    const response = await fetch(`${apiBase}/v1/auth/register`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ email, password, promotion_code: promo }),
    });
    const body = await response.json();
    setMessage(response.ok ? `已注册，渠道 ${body.session?.user?.channel_org_id}` : body.error?.message || "失败");
  }

  async function login() {
    const response = await fetch(`${apiBase}/v1/auth/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ email, password }),
    });
    const body = await response.json();
    setMessage(response.ok ? `欢迎 ${body.session?.user?.email}` : body.error?.message || "失败");
  }

  return (
    <main className="mx-auto flex max-w-md flex-col gap-4 px-6 py-12">
      <h1 className="text-3xl font-semibold">注册 / 登录</h1>
      <input className="rounded bg-slate-900 p-3" placeholder="邮箱" value={email} onChange={(e) => setEmail(e.target.value)} />
      <input className="rounded bg-slate-900 p-3" placeholder="密码（至少 8 位）" type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
      <input className="rounded bg-slate-900 p-3" placeholder="推广码 THA1 / THB1 / THC1" value={promo} onChange={(e) => setPromo(e.target.value)} />
      <div className="flex gap-3">
        <button className="rounded px-4 py-2 text-slate-950" style={{ background: "var(--brand-primary)" }} onClick={register}>
          注册
        </button>
        <button className="rounded border border-slate-600 px-4 py-2" onClick={login}>
          登录
        </button>
      </div>
      <p className="text-sm text-slate-300">{message}</p>
    </main>
  );
}
