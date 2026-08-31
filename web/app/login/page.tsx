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

  return (
    <main className="mx-auto flex min-h-[calc(100vh-8rem)] max-w-md items-center px-6 py-20">
      <section className="w-full rounded-card border border-hairline bg-canvas-raised p-8">
        <p className="th-eyebrow text-brand-emphasis">Sign in</p>
        <h1 className="mt-3 text-[40px] font-semibold leading-tight">注册 / 登录</h1>
        <p className="mt-3 text-base leading-relaxed text-ink-secondary">
          推广码在注册成功时由服务端固化，不能当作可改归属。没有渐变英雄。
        </p>
        <Form {...form}>
          <form className="mt-6 flex flex-col gap-3" onSubmit={(event) => event.preventDefault()}>
            <TextField control={form.control} name="email" label="邮箱" />
            <TextField control={form.control} name="password" label="密码" placeholder="密码（至少 8 位）" type="password" />
            <TextField control={form.control} name="promo" label="推广码" placeholder="推广码 THA1 / THB1 / THC1" />
            <div className="mt-2 flex gap-3">
              <Button type="button" onClick={form.handleSubmit(login)}>
                登录
              </Button>
              <Button type="button" variant="outline" onClick={form.handleSubmit(register)}>
                注册
              </Button>
            </div>
            <p className="text-sm text-hold">{message}</p>
            <p className="text-xs text-ink-mute">
              还没看过价目？先回 <Link href="/">公共站</Link>。
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
