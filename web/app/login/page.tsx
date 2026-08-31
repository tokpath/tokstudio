"use client";

import { Suspense, useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { z } from "zod";
import { TextField } from "@/components/text-field";
import { Button } from "@/components/ui/button";
import { Form } from "@/components/ui/form";
import { apiBase } from "@/lib/api";
import { safeNextPath } from "@/lib/login-next";

const schema = z.object({
  email: z.string().trim().email("请填写有效邮箱"),
  password: z.string().min(8, "密码至少 8 位"),
  promo: z.string().trim(),
  otp: z.string().trim(),
});

function LoginForm() {
  const search = useSearchParams();
  const [message, setMessage] = useState("");
  const [mode, setMode] = useState<"login" | "register">("login");
  const [googleState, setGoogleState] = useState("");
  const [googleEmail, setGoogleEmail] = useState("");
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { email: "", password: "", promo: "", otp: "" },
  });

  function goNext() {
    const next = safeNextPath(search.get("next"));
    window.location.href = next || "/app";
  }

  async function register(values: z.infer<typeof schema>) {
    const response = await fetch(`${apiBase}/v1/auth/register`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ email: values.email, password: values.password, promotion_code: values.promo }),
    });
    const body = await response.json();
    if (response.ok) {
      setMessage(`已注册，渠道 ${body.session?.user?.channel_org_id}`);
      goNext();
      return;
    }
    setMessage(body.error?.message || "失败");
  }

  async function login(values: z.infer<typeof schema>) {
    const response = await fetch(`${apiBase}/v1/auth/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify({ email: values.email, password: values.password }),
    });
    const body = await response.json();
    if (response.ok) {
      setMessage(`欢迎 ${body.session?.user?.email}`);
      goNext();
      return;
    }
    setMessage(body.error?.message || "失败");
  }

  async function googleStart() {
    const response = await fetch(`${apiBase}/v1/auth/google/start`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || "Google 登录不可用");
      return;
    }
    if (body.mock) {
      setGoogleState(body.state || "");
      setGoogleEmail(form.getValues("email"));
      setMessage("开发环境：填写 Google 邮箱后继续（无需真实 OAuth）");
      return;
    }
    if (body.auth_url) {
      window.location.href = body.auth_url;
      return;
    }
    setMessage("Google 登录未返回跳转地址");
  }

  async function googleFinish() {
    const email = googleEmail.trim();
    if (!googleState || !email.includes("@")) {
      setMessage("请填写 Google 邮箱");
      return;
    }
    const response = await fetch(`${apiBase}/v1/auth/google/callback`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ state: googleState, code: `mock:${email}` }),
    });
    const body = await response.json();
    if (response.ok) {
      setMessage(`欢迎 ${body.session?.user?.email}`);
      goNext();
      return;
    }
    setMessage(body.error?.message || "Google 登录失败");
  }

  function githubStart() {
    setMessage("GitHub 登录尚未接入独立 OAuth，请使用邮箱、验证码或 Google。");
  }

  async function requestOtp() {
    const email = form.getValues("email");
    const response = await fetch(`${apiBase}/v1/auth/otp/request`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, purpose: "login" }),
    });
    const body = await response.json();
    setMessage(
      response.ok
        ? body.dev_code
          ? `验证码已发送（开发回显 ${body.dev_code}）`
          : "验证码已发送"
        : body.error?.message || "发送失败",
    );
  }

  async function loginOtp() {
    const values = form.getValues();
    const response = await fetch(`${apiBase}/v1/auth/otp/verify`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email: values.email, code: values.otp }),
    });
    const body = await response.json();
    if (response.ok) {
      setMessage(`欢迎 ${body.session?.user?.email}`);
      goNext();
      return;
    }
    setMessage(body.error?.message || "验证码无效");
  }

  return (
    <main className="mx-auto flex min-h-[calc(100vh-8rem)] max-w-md items-center px-6 py-20">
      <section className="w-full rounded-card border border-hairline bg-canvas-raised p-8">
        <p className="th-eyebrow text-brand-emphasis">Sign in</p>
        <h1 className="mt-3 text-[40px] font-semibold leading-tight">注册 / 登录</h1>
        <p className="mt-3 text-base leading-relaxed text-ink-secondary">
          结构对齐 ofox：GitHub / Google 在上，邮箱密码在下。GitHub 独立 OAuth 尚未接入。忘记密码请用邮件 OTP，没有单独找回页。推广码只在注册时由服务端固化归属。
        </p>

        <div className="mt-6 flex flex-col gap-2">
          <Button type="button" variant="outline" className="w-full" onClick={githubStart}>
            使用 GitHub 登录
          </Button>
          <Button type="button" variant="outline" className="w-full" onClick={googleStart}>
            使用 Google 登录
          </Button>
        </div>
        {googleState ? (
          <div className="mt-3 flex flex-col gap-2 rounded-card border border-hairline bg-canvas p-3">
            <label className="text-[13px] text-ink-secondary" htmlFor="google-email">
              Google 邮箱（开发 mock）
            </label>
            <input
              id="google-email"
              className="rounded-control border border-hairline bg-canvas-raised px-3 py-2 text-sm"
              value={googleEmail}
              onChange={(event) => setGoogleEmail(event.target.value)}
              placeholder="you@gmail.com"
              type="email"
            />
            <Button type="button" variant="outline" onClick={googleFinish}>
              继续 Google 登录
            </Button>
          </div>
        ) : null}

        <div className="my-6 flex items-center gap-3 text-[13px] text-ink-mute">
          <span className="h-px flex-1 bg-hairline" />
          或
          <span className="h-px flex-1 bg-hairline" />
        </div>

        <Form {...form}>
          <form className="flex flex-col gap-3" onSubmit={(event) => event.preventDefault()}>
            <TextField control={form.control} name="email" label="邮箱" placeholder="m@example.com" />
            <TextField control={form.control} name="password" label="密码" placeholder="请输入密码（至少 8 位）" type="password" />
            {mode === "login" ? (
              <div className="flex flex-wrap items-end gap-2">
                <TextField control={form.control} name="otp" label="邮箱验证码" placeholder="可选 · 邮件 OTP" className="flex-1" />
                <Button type="button" variant="ghost" onClick={requestOtp}>
                  发送验证码
                </Button>
              </div>
            ) : null}
            {mode === "register" ? (
              <TextField control={form.control} name="promo" label="推广码" placeholder="可选 · THA1 / THB1 / THC1" />
            ) : null}
            <Button
              type="button"
              className="mt-2 w-full"
              onClick={form.handleSubmit(mode === "login" ? login : register)}
            >
              {mode === "login" ? "登录" : "注册"}
            </Button>
            {mode === "login" ? (
              <>
                <Button type="button" variant="outline" className="w-full" onClick={loginOtp}>
                  用验证码登录
                </Button>
                <p className="text-[12px] text-ink-mute">忘记密码？发送验证码后点「用验证码登录」。没有独立找回密码接口。</p>
              </>
            ) : null}
            <p className="text-sm text-hold">{message}</p>
            <p className="text-sm text-ink-secondary">
              {mode === "login" ? (
                <>
                  还没有账户？{" "}
                  <button type="button" className="text-brand-emphasis" onClick={() => setMode("register")}>
                    注册
                  </button>
                </>
              ) : (
                <>
                  已有账户？{" "}
                  <button type="button" className="text-brand-emphasis" onClick={() => setMode("login")}>
                    登录
                  </button>
                </>
              )}
            </p>
            <p className="text-[12px] text-ink-mute">
              继续即表示你了解 <Link href="/terms">服务条款</Link> 与 <Link href="/privacy">隐私政策</Link>。还没看过价目？先回{" "}
              <Link href="/">公共站</Link>。
            </p>
          </form>
        </Form>
      </section>
    </main>
  );
}

export default function LoginPage() {
  return (
    <Suspense fallback={<main className="mx-auto max-w-md px-6 py-20 text-ink-secondary">登录 / 注册</main>}>
      <LoginForm />
    </Suspense>
  );
}
