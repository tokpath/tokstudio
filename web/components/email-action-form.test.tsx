/** @vitest-environment jsdom */
import { StrictMode } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { withZh } from "@/lib/test-i18n";
import { EmailActionForm } from "./email-action-form";
import { EmailVerificationStatus } from "./email-verification-status";

vi.mock("next/navigation", () => ({ useSearchParams: () => new URLSearchParams() }));
vi.mock("@/components/auth-card", () => ({ AuthCard: ({ title, children }: { title: string; children: React.ReactNode }) => <main><h1>{title}</h1>{children}</main> }));
const token = `themail_${"a".repeat(48)}`;

describe("email account flows", () => {
  beforeEach(() => { window.history.replaceState(null, "", "/forgot-password"); vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ ok: true, retry_after: 60 }), { status: 202 }))); });
  afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

  it("keeps a generic reset confirmation and disables resending during cooldown", async () => {
    render(withZh(<EmailActionForm kind="forgot" />));
    fireEvent.change(await screen.findByLabelText("邮箱"), { target: { value: "unknown@example.test" } });
    fireEvent.click(screen.getByRole("button", { name: "发送邮件" }));
    expect(await screen.findByRole("status")).toHaveProperty("textContent", expect.stringContaining("如果该邮箱在当前品牌存在可用账号"));
    expect(screen.getByRole("button", { name: "60 秒后可重发" })).toHaveProperty("disabled", true);
    expect(fetch).toHaveBeenCalledWith("/api/v1/auth/password/forgot", expect.objectContaining({ body: JSON.stringify({ email: "unknown@example.test" }) }));
  });

  it("preserves the token during Strict Mode and reloads, and clears it after success", async () => {
    window.history.replaceState(null, "", `/reset-password#token=${token}`);
    render(withZh(<StrictMode><EmailActionForm kind="reset" /></StrictMode>));
    fireEvent.change(await screen.findByLabelText("新密码"), { target: { value: "new-password" } });
    fireEvent.change(screen.getByLabelText("确认新密码"), { target: { value: "different-password" } });
    fireEvent.click(screen.getByRole("button", { name: "重置密码" }));
    expect(await screen.findByRole("alert")).toHaveProperty("textContent", expect.stringContaining("两次输入的密码不一致"));
    expect(fetch).not.toHaveBeenCalled(); expect(window.location.hash).toContain(token);
    fireEvent.change(screen.getByLabelText("确认新密码"), { target: { value: "new-password" } });
    fireEvent.click(screen.getByRole("button", { name: "重置密码" }));
    expect(await screen.findByRole("status")).toHaveProperty("textContent", expect.stringContaining("密码已更新"));
    expect(window.location.hash).toBe("");
    expect(fetch).toHaveBeenCalledWith("/api/v1/auth/password/reset", expect.objectContaining({ body: JSON.stringify({ token, new_password: "new-password" }) }));
    expect(screen.getByRole("link", { name: "返回登录" }).getAttribute("href")).toBe("/login");
  });

  it("requires an explicit verification click and lets an expired link request a new email", async () => {
    window.history.replaceState(null, "", `/verify-email#token=${token}`);
    vi.stubGlobal("fetch", vi.fn(async (url: string) => new Response(JSON.stringify(url.endsWith("/verify") ? { error: { code: "email_link_invalid" } } : { ok: true, retry_after: 60 }), { status: url.endsWith("/verify") ? 400 : 202 })));
    render(withZh(<EmailActionForm kind="verify" />));
    const confirm = await screen.findByRole("button", { name: "确认验证邮箱" });
    expect(fetch).not.toHaveBeenCalled();
    fireEvent.click(confirm); await screen.findByRole("alert");
    fireEvent.click(screen.getByRole("button", { name: "重新申请邮件" }));
    fireEvent.change(screen.getByLabelText("邮箱"), { target: { value: "member@example.test" } });
    fireEvent.click(screen.getByRole("button", { name: "发送邮件" }));
    expect(await screen.findByRole("status")).toHaveProperty("textContent", expect.stringContaining("待验证"));
  });

  it("shows unavailable mail delivery without claiming an email was sent", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ error: { code: "email_unavailable" } }), { status: 503 })));
    render(withZh(<EmailActionForm kind="forgot" />));
    fireEvent.change(await screen.findByLabelText("邮箱"), { target: { value: "member@example.test" } });
    fireEvent.click(screen.getByRole("button", { name: "发送邮件" }));
    expect(await screen.findByRole("alert")).toHaveProperty("textContent", expect.stringContaining("邮件服务暂不可用"));
    expect(screen.queryByText(/如果该邮箱在当前品牌存在可用账号/)).toBeNull();
  });

  it("refreshes verification after the email is confirmed in another browser", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ user: { email_verified: true } }))));
    render(withZh(<EmailVerificationStatus email="member@example.test" verified={false} />));
    fireEvent.click(screen.getByRole("button", { name: "刷新验证状态" }));
    await waitFor(() => expect(screen.queryByRole("button", { name: "重发验证邮件" })).toBeNull());
    expect(screen.getByText("邮箱已验证 · member@example.test")).toBeTruthy();
  });
});
