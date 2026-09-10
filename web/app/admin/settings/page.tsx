"use client";

import { useState } from "react";
import { ConfirmButton } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan, IfRoles } from "@/components/rbac/if-can";

export default function AdminSettingsPage() {
  const [rate, setRate] = useState("0.5");
  const [minReq, setMinReq] = useState("5");
  const [pending, setPending] = useState("1");
  const [providerID, setProviderID] = useState("prd_echo_primary");
  const [canarySlug, setCanarySlug] = useState("echo-backup");
  const [canaryPercent, setCanaryPercent] = useState("0");
  const [brandID, setBrandID] = useState("brd_oem");
  const [message, setMessage] = useState("告警阈值写入 ops 表，评估成功率时会读取。");
  const [drillMessage, setDrillMessage] = useState("支付/媒体/TLS 演练不强制确认头。TLS 只验沙箱门禁，不是公网 ACME。");
  const [totpMessage, setTotpMessage] = useState("读取状态不回密文。绑定后用 6 位码确认启用。不要在共享管理员上留下 enabled。");
  const [totpStatus, setTotpStatus] = useState("disabled");
  const [totpSecret, setTotpSecret] = useState("");
  const [totpURL, setTotpURL] = useState("");
  const [totpCode, setTotpCode] = useState("");

  async function load2FA() {
    const res = await fetch(`${apiBase}/admin/me/2fa`, { credentials: "include" });
    const body = await res.json();
    if (!res.ok) {
      setTotpMessage(body.error?.message || "读取 2FA 失败");
      return;
    }
    setTotpStatus(String(body.item?.status || "disabled"));
    setTotpMessage(`状态 ${body.item?.status || "disabled"} / enabled=${body.item?.enabled ? "true" : "false"}`);
  }

  async function setup2FA() {
    const res = await fetch(`${apiBase}/admin/me/2fa/setup`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: "{}",
    });
    const body = await res.json();
    if (!res.ok) {
      setTotpMessage(body.error?.message || "绑定失败");
      return;
    }
    setTotpStatus(String(body.item?.status || "pending"));
    setTotpSecret(String(body.item?.secret || ""));
    setTotpURL(String(body.item?.otpauth_url || ""));
    setTotpMessage(`已绑定 pending，用验证器扫 otpauth 后再点确认启用`);
  }

  async function enable2FA(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    const code = String(data.get("enable_code") || "").trim();
    const res = await fetch(`${apiBase}/admin/me/2fa/enable`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ code }),
    });
    const body = await res.json();
    if (!res.ok) {
      setTotpMessage(body.error?.message || "启用失败");
      return;
    }
    form.reset();
    setTotpStatus("enabled");
    setTotpSecret("");
    setTotpURL("");
    setTotpMessage("已启用。共享管理员请立刻关闭，否则后续写操作都会要 TOTP。");
  }

  async function disable2FA(code: string) {
    const headers: Record<string, string> = { ...confirmHeaders };
    if (code) {
      headers["X-Tokenhub-TOTP"] = code;
    }
    const res = await fetch(`${apiBase}/admin/me/2fa/disable`, {
      method: "POST",
      credentials: "include",
      headers,
      body: "{}",
    });
    const body = await res.json();
    if (!res.ok) {
      setTotpMessage(body.error?.message || "关闭失败");
      return;
    }
    setTotpCode("");
    setTotpStatus(String(body.status || "disabled"));
    setTotpSecret("");
    setTotpURL("");
    setTotpMessage("已关闭 2FA。敏感写操作不再要 TOTP。");
  }
  function setLocale(locale: string) {
    if (locale === "auto") {
      document.cookie = "NEXT_LOCALE=; path=/; max-age=0";
    } else {
      document.cookie = `NEXT_LOCALE=${locale}; path=/; max-age=31536000`;
    }
    window.location.reload();
  }
  async function loadThresholds() {
    const res = await fetch(`${apiBase}/admin/ops/thresholds`, { credentials: "include" });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "未登录平台管理员");
      return;
    }
    setRate(String(body.thresholds?.success_rate_min ?? 0.5));
    setMinReq(String(body.thresholds?.min_requests ?? 5));
    setPending(String(body.thresholds?.pending_count ?? 1));
    setMessage("已读取告警阈值");
  }
  async function saveThresholds() {
    const res = await fetch(`${apiBase}/admin/ops/thresholds`, {
      method: "PATCH",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({
        success_rate_min: Number(rate),
        min_requests: Number(minReq),
        pending_count: Number(pending),
      }),
    });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || "保存失败");
      return;
    }
    setMessage(`已保存：成功率 < ${body.thresholds?.success_rate_min} 且请求 ≥ ${body.thresholds?.min_requests}`);
  }
  return (
    <AdminShell>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="settings" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-4 text-sm text-ink-secondary">管理员 2FA 使用 TOTP。语言预留中 / 英 / 日（next-intl）。</p>
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" onClick={() => setLocale("auto")}>
            自动
          </Button>
          <Button type="button" variant="outline" onClick={() => setLocale("zh")}>
            中文
          </Button>
          <Button type="button" variant="outline" onClick={() => setLocale("en")}>
            English
          </Button>
          <Button type="button" variant="outline" onClick={() => setLocale("ja")}>
            日本語
          </Button>
        </div>
      </section>
      <IfCan action="settings.totp">
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="twofa" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">
          读取状态不回密文。开始绑定后用验证器扫码，再填 6 位码确认启用。关闭要二次确认；已经 enabled 时还要带 TOTP。不要在共享管理员上留下 enabled。
        </p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Button size="sm" variant="outline" onClick={load2FA}>
            读取 2FA
          </Button>
          <Button size="sm" variant="outline" onClick={setup2FA}>
            开始绑定
          </Button>
        </div>
        <p className="mb-3 text-sm text-ink-secondary">当前 {totpStatus}</p>
        {totpSecret ? <p className="mb-3 break-all text-xs text-ink-secondary">secret={totpSecret}</p> : null}
        {totpURL ? <p className="mb-3 break-all text-xs text-ink-secondary">{totpURL}</p> : null}
        <form className="mb-3 flex flex-wrap gap-2" onSubmit={enable2FA}>
          <Input name="enable_code" aria-label="启用用 TOTP" placeholder="启用用 6 位码" />
          <Button size="sm" type="submit">
            确认启用
          </Button>
        </form>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input
            name="disable_code"
            aria-label="关闭用 TOTP"
            placeholder="关闭用 6 位码（enabled 时必填）"
            value={totpCode}
            onChange={(e) => setTotpCode(e.target.value)}
          />
          <ConfirmButton size="sm" title="确认关闭 2FA" description="关闭后敏感写操作不再要 TOTP。已经 enabled 时还要带验证码。" onConfirm={() => disable2FA(totpCode.trim())}>
            关闭 2FA
          </ConfirmButton>
        </div>
        <p className="text-sm text-ink-secondary">{totpMessage}</p>
      </section>
      </IfCan>
      <IfCan action="settings.thresholds.view">
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="thresholds" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">成功率下限、最少请求数、待对账条数。保存需要二次确认头。</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-28" value={rate} onChange={(e) => setRate(e.target.value)} aria-label="成功率下限" />
          <Input className="w-28" value={minReq} onChange={(e) => setMinReq(e.target.value)} aria-label="最少请求数" />
          <Input className="w-28" value={pending} onChange={(e) => setPending(e.target.value)} aria-label="待对账条数" />
          <Button type="button" variant="outline" onClick={loadThresholds}>
            读取阈值
          </Button>
          <IfCan action="settings.thresholds">
          <ConfirmButton size="sm" variant="outline" title="确认保存阈值" description="评估告警时会读取这些阈值。" onConfirm={saveThresholds}>
            保存阈值
          </ConfirmButton>
          </IfCan>
        </div>
        <p className="text-sm text-ink-secondary">{message}</p>
      </section>
      </IfCan>
      <IfRoles roles={["platform_admin", "ops_admin", "tech_admin"]}>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="ops" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">健康探测不会计费。熔断跳过该 Provider；灰度按百分比把带 X-Tokenhub-Canary 的流量切到指定 slug。</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-56" value={providerID} onChange={(e) => setProviderID(e.target.value)} aria-label="Provider ID" />
          <IfCan action="settings.circuit">
          <Button
            size="sm"
            variant="outline"
            onClick={async () => {
              const res = await fetch(`${apiBase}/admin/providers/${providerID}/health-check`, {
                method: "POST",
                credentials: "include",
                headers: { "Content-Type": "application/json" },
                body: "{}",
              });
              const body = await res.json();
              setMessage(res.ok ? `健康=${body.health}` : body.error?.message || "探测失败");
            }}
          >
            健康探测
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={async () => {
              const res = await fetch(`${apiBase}/admin/ops/circuit/${providerID}`, {
                method: "POST",
                credentials: "include",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ action: "trip" }),
              });
              const body = await res.json();
              setMessage(res.ok ? `已打开熔断 ${providerID}` : body.error?.message || "熔断失败");
            }}
          >
            打开熔断
          </Button>
          <Button
            size="sm"
            onClick={async () => {
              const res = await fetch(`${apiBase}/admin/ops/circuit/${providerID}`, {
                method: "POST",
                credentials: "include",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ action: "reset" }),
              });
              const body = await res.json();
              setMessage(res.ok ? `已复位熔断 ${providerID}` : body.error?.message || "复位失败");
            }}
          >
            复位熔断
          </Button>
          </IfCan>
        </div>
        <div className="flex flex-wrap gap-2">
          <Input className="w-40" value={canarySlug} onChange={(e) => setCanarySlug(e.target.value)} aria-label="灰度 Provider slug" />
          <Input className="w-24" value={canaryPercent} onChange={(e) => setCanaryPercent(e.target.value)} aria-label="灰度百分比" />
          <Button
            size="sm"
            variant="outline"
            onClick={async () => {
              const res = await fetch(`${apiBase}/admin/ops/canary`, { credentials: "include" });
              const body = await res.json();
              if (!res.ok) {
                setMessage(body.error?.message || "读取灰度失败");
                return;
              }
              setCanarySlug(body.canary?.provider_slug || "");
              setCanaryPercent(String(body.canary?.percent ?? 0));
              setMessage(`灰度 ${body.canary?.provider_slug || "-"} ${body.canary?.percent ?? 0}%`);
            }}
          >
            读取灰度
          </Button>
          <Button
            size="sm"
            onClick={async () => {
              const res = await fetch(`${apiBase}/admin/ops/canary`, {
                method: "POST",
                credentials: "include",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ provider_slug: canarySlug, percent: Number(canaryPercent) }),
              });
              const body = await res.json();
              setMessage(res.ok ? `已设置灰度 ${body.canary?.provider_slug} ${body.canary?.percent}%` : body.error?.message || "设置失败");
            }}
          >
            保存灰度
          </Button>
        </div>
      </section>
      </IfRoles>
      <IfCan action="settings.backup">
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="backup" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">只验证 Postgres / Redis / migration，并记录 RPO 15 分钟、RTO 1 小时。不是把整库真的恢复一遍。</p>
        <Button
          size="sm"
          onClick={async () => {
            const res = await fetch(`${apiBase}/admin/ops/backup-drill`, {
              method: "POST",
              credentials: "include",
              headers: { "Content-Type": "application/json" },
              body: "{}",
            });
            const body = await res.json();
            setMessage(
              res.ok
                ? `演练 ${body.item?.status || "ok"}：RPO ${body.item?.rpo_minutes} 分钟 / RTO ${body.item?.rto_minutes} 分钟`
                : body.error?.message || "演练失败",
            );
          }}
        >
          备份演练
        </Button>
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </section>
      </IfCan>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="drill" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">
          支付演练必须拒绝伪造签名；媒体演练只记录 force-fail 必须释放预授权；TLS 演练核对已知域名 200、未知 404、沙箱 issued。
        </p>
        <div className="flex flex-wrap items-center gap-2">
          <IfCan action="settings.drill.payment">
          <Button
            size="sm"
            variant="outline"
            onClick={async () => {
              const res = await fetch(`${apiBase}/admin/ops/drills/payment`, {
                method: "POST",
                credentials: "include",
                headers: { "Content-Type": "application/json" },
                body: "{}",
              });
              const body = await res.json();
              setDrillMessage(res.ok ? `支付演练 ${body.item?.status}：${body.item?.detail}` : body.error?.message || "支付演练失败");
            }}
          >
            支付演练
          </Button>
          </IfCan>
          <IfCan action="settings.drill.media">
          <Button
            size="sm"
            variant="outline"
            onClick={async () => {
              const res = await fetch(`${apiBase}/admin/ops/drills/media`, {
                method: "POST",
                credentials: "include",
                headers: { "Content-Type": "application/json" },
                body: "{}",
              });
              const body = await res.json();
              setDrillMessage(res.ok ? `媒体演练 ${body.item?.status}：${body.item?.detail}` : body.error?.message || "媒体演练失败");
            }}
          >
            媒体演练
          </Button>
          </IfCan>
          <IfCan action="settings.drill.tls">
          <Button
            size="sm"
            onClick={async () => {
              const res = await fetch(`${apiBase}/admin/ops/drills/tls`, {
                method: "POST",
                credentials: "include",
                headers: { "Content-Type": "application/json" },
                body: "{}",
              });
              const body = await res.json();
              setDrillMessage(res.ok ? `TLS 演练 ${body.item?.status}：${body.item?.detail}` : body.error?.message || "TLS 演练失败");
            }}
          >
            TLS 演练
          </Button>
          </IfCan>
          <p className="text-sm text-ink-secondary">{drillMessage}</p>
        </div>
      </section>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="oem" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">.localhost / 空目录走沙箱 issued。配齐 Cloudflare API Token 与 Zone ID 后，公网形态域名会登记 Custom Hostname；Worker 会刷新 pending。客户把域名 CNAME 到入口。未登记域名 tls-check 仍 404。不自建公网 Let&apos;s Encrypt。</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-56" value={brandID} onChange={(e) => setBrandID(e.target.value)} aria-label="品牌 ID" placeholder="brd_oem" />
          <Button
            size="sm"
            variant="outline"
            onClick={async () => {
              const res = await fetch(`${apiBase}/admin/brands`, { credentials: "include" });
              const body = await res.json();
              if (!res.ok) {
                setMessage(body.error?.message || "读取品牌失败");
                return;
              }
              const items = body.items || [];
              const hit = items.find((item: { id?: string }) => item.id === brandID) || items[0];
              setMessage(hit ? `${hit.id} CNAME=${hit.cname_target || "-"} TLS=${hit.tls_status || "pending"} issuer=${hit.tls_issuer || "sandbox"}` : "没有品牌");
            }}
          >
            读取品牌
          </Button>
          <IfCan action="settings.drill.tls">
          <ConfirmButton
            size="sm"
            title="确认签发证书"
            description="沙箱域名只标 issued。已配置 Cloudflare 的公网形态域名会登记 Custom Hostname。Pebble 仅本地演练 RFC 8555。"
            onConfirm={async () => {
              const res = await fetch(`${apiBase}/admin/brands/${brandID}/tls/issue`, {
                method: "POST",
                credentials: "include",
                headers: confirmHeaders,
                body: "{}",
              });
              const body = await res.json();
              setMessage(
                res.ok
                  ? `已签发 ${body.item?.id} → ${body.item?.tls_status} / ${body.item?.cname_target} / ${body.item?.tls_issuer || "sandbox"}`
                  : body.error?.message || "签发失败",
              );
            }}
          >
            签发证书
          </ConfirmButton>
          </IfCan>
        </div>
        <p className="text-sm text-ink-secondary">{message}</p>
      </section>
    </AdminShell>
  );
}
