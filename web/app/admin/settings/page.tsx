"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";

export default function AdminSettingsPage() {
  const [rate, setRate] = useState("0.5");
  const [minReq, setMinReq] = useState("5");
  const [pending, setPending] = useState("1");
  const [providerID, setProviderID] = useState("prd_echo_primary");
  const [canarySlug, setCanarySlug] = useState("echo-backup");
  const [canaryPercent, setCanaryPercent] = useState("0");
  const [brandID, setBrandID] = useState("brd_oem");
  const [message, setMessage] = useState("告警阈值写入 ops 表，评估成功率时会读取。");

  async function setup2FA() {
    await apiClient("POST", "/admin/me/2fa/setup", { method: "POST" });
  }
  function setLocale(locale: string) {
    document.cookie = `NEXT_LOCALE=${locale}; path=/; max-age=31536000`;
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
      headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
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
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-3 text-xl font-medium">系统设置</h2>
        <p className="mb-4 text-sm text-slate-400">管理员 2FA 使用 TOTP。语言预留中 / 英 / 日（next-intl）。</p>
        <div className="flex flex-wrap gap-2">
          <button className="rounded border border-slate-600 px-3 py-2" onClick={setup2FA}>
            启用 2FA
          </button>
          <button className="rounded border border-slate-600 px-3 py-2" onClick={() => setLocale("zh")}>
            中文
          </button>
          <button className="rounded border border-slate-600 px-3 py-2" onClick={() => setLocale("en")}>
            English
          </button>
          <button className="rounded border border-slate-600 px-3 py-2" onClick={() => setLocale("ja")}>
            日本語
          </button>
        </div>
      </section>
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-3 text-xl font-medium">告警阈值</h2>
        <p className="mb-3 text-sm text-slate-400">成功率下限、最少请求数、待对账条数。保存需要二次确认头。</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <input className="w-28 rounded bg-slate-900 p-2" value={rate} onChange={(e) => setRate(e.target.value)} aria-label="成功率下限" />
          <input className="w-28 rounded bg-slate-900 p-2" value={minReq} onChange={(e) => setMinReq(e.target.value)} aria-label="最少请求数" />
          <input className="w-28 rounded bg-slate-900 p-2" value={pending} onChange={(e) => setPending(e.target.value)} aria-label="待对账条数" />
          <button className="rounded border border-slate-600 px-3 py-2" onClick={loadThresholds}>
            读取阈值
          </button>
          <button className="rounded border border-slate-600 px-3 py-2" onClick={saveThresholds}>
            保存阈值
          </button>
        </div>
        <p className="text-sm text-slate-300">{message}</p>
      </section>
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-3 text-xl font-medium">运维开关</h2>
        <p className="mb-3 text-sm text-slate-400">健康探测不会计费。熔断跳过该 Provider；灰度按百分比把带 X-Tokenhub-Canary 的流量切到指定 slug。</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-56" value={providerID} onChange={(e) => setProviderID(e.target.value)} aria-label="Provider ID" />
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
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-3 text-xl font-medium">备份演练</h2>
        <p className="mb-3 text-sm text-slate-400">只验证 Postgres / Redis / migration，并记录 RPO 15 分钟、RTO 1 小时。不是把整库真的恢复一遍。</p>
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
        <p className="mt-3 text-sm text-slate-300">{message}</p>
      </section>
      <section className="rounded-2xl border border-slate-800 bg-slate-900/70 p-6">
        <h2 className="mb-3 text-xl font-medium">OEM 证书</h2>
        <p className="mb-3 text-sm text-slate-400">沙箱把 tls_status 标成 issued，并写下 CNAME。公网 Let&apos;s Encrypt 仍由边缘节点签发。</p>
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
              setMessage(hit ? `${hit.id} CNAME=${hit.cname_target || "-"} TLS=${hit.tls_status || "pending"}` : "没有品牌");
            }}
          >
            读取品牌
          </Button>
          <Button
            size="sm"
            onClick={async () => {
              const res = await fetch(`${apiBase}/admin/brands/${brandID}/tls/issue`, {
                method: "POST",
                credentials: "include",
                headers: { "Content-Type": "application/json", "X-Tokenhub-Confirm": "1" },
                body: "{}",
              });
              const body = await res.json();
              setMessage(
                res.ok
                  ? `已签发 ${body.item?.id} → ${body.item?.tls_status} / ${body.item?.cname_target}`
                  : body.error?.message || "签发失败",
              );
            }}
          >
            签发证书
          </Button>
        </div>
        <p className="text-sm text-slate-300">{message}</p>
      </section>
    </AdminShell>
  );
}
