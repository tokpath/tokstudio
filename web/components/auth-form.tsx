"use client";

import { useState, type FormEvent } from "react";
import { Button } from "@/components/button";
import { Eyebrow } from "@/components/eyebrow";

const HOLD_COPY = "身份服务尚未接通。渠道归属会在注册成功时由服务端固化，不能在此改归属。";

export function AuthForm({ mode }: { mode: "login" | "register" }) {
  const [message, setMessage] = useState("");

  function onSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setMessage(HOLD_COPY);
  }

  return (
    <form
      onSubmit={onSubmit}
      className="mx-auto flex w-full max-w-md flex-col gap-6 rounded-stamp border border-hairline bg-canvas-raised p-8"
    >
      <div className="flex flex-col gap-2">
        <Eyebrow className="text-brand-emphasis">{mode === "login" ? "Sign in" : "Register"}</Eyebrow>
        <h1 className="text-2xl font-semibold">{mode === "login" ? "登录" : "注册"}</h1>
        <p className="text-sm text-ink-secondary">
          邮箱密码、验证码或 Google。没有渐变英雄，也没有可改的推广归属。
        </p>
      </div>
      <label className="flex flex-col gap-2 text-sm text-ink">
        邮箱
        <input
          type="email"
          name="email"
          required
          autoComplete="email"
          className="min-h-10 rounded-control border border-hairline bg-canvas px-3 text-base text-ink"
        />
      </label>
      <label className="flex flex-col gap-2 text-sm text-ink">
        密码
        <input
          type="password"
          name="password"
          required
          autoComplete={mode === "login" ? "current-password" : "new-password"}
          className="min-h-10 rounded-control border border-hairline bg-canvas px-3 font-mono text-base text-ink"
        />
      </label>
      <button
        type="submit"
        className="inline-flex min-h-10 items-center justify-center rounded-stamp bg-brand px-4 text-sm font-medium text-on-brand hover:bg-brand-press max-sm:min-h-11"
      >
        {mode === "login" ? "登录" : "创建账号"}
      </button>
      <Button type="button" variant="secondary" onClick={() => setMessage(HOLD_COPY)}>
        使用 Google 继续
      </Button>
      {message ? <p className="text-sm text-hold">{message}</p> : null}
      <p className="text-sm text-ink-mute">
        {mode === "login" ? (
          <>
            还没有账号？<a href="/register">注册</a>
          </>
        ) : (
          <>
            已有账号？<a href="/login">登录</a>
          </>
        )}
      </p>
    </form>
  );
}
