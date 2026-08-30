"use client";

import { Suspense, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";
import { safeNextPath } from "@/lib/login-next";

function LoginForm() {
  const search = useSearchParams();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [promo, setPromo] = useState("");
  const [message, setMessage] = useState("");

  function goNext() {
    const next = safeNextPath(search.get("next"));
    window.location.href = next || "/app";
  }

  async function register() {
    const response = await fetch(`${apiBase}/v1/auth/register`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ email, password, promotion_code: promo }),
    });
    const body = await response.json();
    if (response.ok) {
      setMessage(`已注册，渠道 ${body.session?.user?.channel_org_id}`);
      goNext();
      return;
    }
    setMessage(body.error?.message || "失败");
  }

  async function login() {
    const response = await fetch(`${apiBase}/v1/auth/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ email, password }),
    });
    const body = await response.json();
    if (response.ok) {
      setMessage(`欢迎 ${body.session?.user?.email}`);
      goNext();
      return;
    }
    setMessage(body.error?.message || "失败");
  }

  return (
    <main className="mx-auto grid min-h-[calc(100vh-8rem)] max-w-6xl items-center gap-10 px-6 py-12 lg:grid-cols-2">
      <div className="hidden lg:block">
        <p className="text-sm uppercase tracking-[0.2em]" style={{ color: "var(--brand-primary)" }}>
          开始使用
        </p>
        <h2 className="mt-3 text-4xl font-semibold tracking-tight">一把 Key，接进控制台</h2>
        <p className="mt-4 max-w-md text-slate-400">
          登录成功后会回到刚才的购买页或用户控制台。推广码决定渠道归属，注册后不能自己改。
        </p>
        <ul className="mt-8 space-y-3 text-sm text-slate-300">
          <li>· 用户控制台管余额、Key、用量和媒体任务</li>
          <li>· 渠道控制台只看本渠道数据和套餐</li>
          <li>· 平台管理看提供商、价格、佣金和审计</li>
        </ul>
      </div>
      <section className="rounded-3xl border border-white/10 bg-white/[0.035] p-8 shadow-glow">
        <h1 className="text-3xl font-semibold tracking-tight">注册 / 登录</h1>
        <p className="mt-2 text-sm text-slate-400">登录成功后会回到刚才的购买页或用户控制台。</p>
        <div className="mt-6 flex flex-col gap-3">
          <label className="text-xs text-slate-500">邮箱</label>
          <Input placeholder="邮箱" value={email} onChange={(e) => setEmail(e.target.value)} />
          <label className="text-xs text-slate-500">密码</label>
          <Input placeholder="密码（至少 8 位）" type="password" value={password} onChange={(e) => setPassword(e.target.value)} />
          <label className="text-xs text-slate-500">推广码</label>
          <Input placeholder="推广码 THA1 / THB1 / THC1" value={promo} onChange={(e) => setPromo(e.target.value)} />
          <div className="mt-2 flex gap-3">
            <Button onClick={register}>注册</Button>
            <Button variant="outline" onClick={login}>
              登录
            </Button>
          </div>
          <p className="text-sm text-slate-300">{message}</p>
          <p className="text-xs text-slate-500">
            还没看过模型目录？先回 <Link href="/" className="underline">公共站</Link>。
          </p>
        </div>
      </section>
    </main>
  );
}

export default function LoginPage() {
  return (
    <Suspense fallback={<main className="mx-auto max-w-md px-6 py-12 text-slate-300">注册 / 登录</main>}>
      <LoginForm />
    </Suspense>
  );
}
