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
});

function LoginForm() {
  const search = useSearchParams();
  const [message, setMessage] = useState("");
  const [mode, setMode] = useState<"login" | "register">("login");
  const form = useForm<z.infer<typeof schema>>({
    resolver: zodResolver(schema),
    defaultValues: { email: "", password: "", promo: "" },
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
    window.location.href = `${apiBase}/v1/auth/google/start`;
  }

  return (
    <main className="mx-auto flex min-h-[calc(100vh-8rem)] max-w-md items-center px-6 py-16">
      <section className="w-full rounded-stamp border border-hairline bg-canvas-raised p-8">
        <p className="th-eyebrow text-brand-emphasis">AUTH</p>
        <h1 className="mt-2 text-2xl font-semibold">{mode === "login" ? "登录" : "注册"}</h1>
        <p className="mt-2 text-sm text-ink-secondary">
          对齐 ofox 控制台登录结构：邮箱密码为主，Google 为次按钮。推广码只在注册时写入归属。
        </p>

        <div className="mt-6 flex flex-col gap-2">
          <Button type="button" variant="outline" className="w-full" onClick={googleStart}>
            使用 Google 登录
          </Button>
        </div>

        <div className="my-6 flex items-center gap-3 text-[13px] text-ink-mute">
          <span className="h-px flex-1 bg-hairline" />
          或
          <span className="h-px flex-1 bg-hairline" />
        </div>

        <Form {...form}>
          <form className="flex flex-col gap-3" onSubmit={(event) => event.preventDefault()}>
            <TextField control={form.control} name="email" label="邮箱" placeholder="m@example.com" />
            <TextField control={form.control} name="password" label="密码" placeholder="请输入密码（至少 8 位）" type="password" />
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
              继续即表示你了解 <Link href="/terms">服务条款</Link> 与 <Link href="/privacy">隐私政策</Link>。
            </p>
          </form>
        </Form>
      </section>
    </main>
  );
}

export default function LoginPage() {
  return (
    <Suspense fallback={<main className="mx-auto max-w-md px-6 py-12 text-ink-secondary">登录 / 注册</main>}>
      <LoginForm />
    </Suspense>
  );
}
