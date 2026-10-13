"use client";

import { networkUnavailable } from "@/lib/confirm";
import { useTranslations } from "next-intl";
import { useState } from "react";
import { ConfirmButton } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AdminShell } from "../shell";
import { apiBase } from "@/lib/api";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { AdminH2 } from "@/components/admin-h2";
import { IfCan, IfRoles } from "@/components/rbac/if-can";
import { NotificationAdminSettings } from "@/components/notifications/admin-settings";

export default function AdminSettingsPage() {
  const tUI = useTranslations();
  const confirmNetworkUnavailable = networkUnavailable(tUI);
  const [rate, setRate] = useState("0.5");
  const [minReq, setMinReq] = useState("5");
  const [pending, setPending] = useState("1");
  const [providerID, setProviderID] = useState("prd_echo_primary");
  const [canarySlug, setCanarySlug] = useState("echo-backup");
  const [canaryPercent, setCanaryPercent] = useState("0");
  const [brandID, setBrandID] = useState("brd_oem");
  const [message, setMessage] = useState(tUI("consoleCopy.alertThresholdsAreStoredInTheOpsTable"));
  const [drillMessage, setDrillMessage] = useState(tUI("consoleCopy.paymentMediaAndTlsDrillsRequireNoConfirmation"));
  const [totpMessage, setTotpMessage] = useState(tUI("consoleCopy.statusReadsNeverReturnSecretsConfirmEnablingWith"));
  const [totpStatus, setTotpStatus] = useState("disabled");
  const [totpSecret, setTotpSecret] = useState("");
  const [totpURL, setTotpURL] = useState("");
  const [totpCode, setTotpCode] = useState("");

  async function load2FA() {
    const res = await fetch(`${apiBase}/admin/me/2fa`, { credentials: "include" });
    const body = await res.json();
    if (!res.ok) {
      setTotpMessage(body.error?.message || tUI("consoleCopy.couldNotRead2faStatus"));
      return;
    }
    setTotpStatus(String(body.item?.status || "disabled"));
    setTotpMessage(tUI("consoleCopy.statusEnabled", { value0: String(body.item?.status || "disabled"), value1: String(body.item?.enabled ? "true" : "false") }));
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
      setTotpMessage(body.error?.message || tUI("consoleCopy.setupFailed"));
      return;
    }
    setTotpStatus(String(body.item?.status || "pending"));
    setTotpSecret(String(body.item?.secret || ""));
    setTotpURL(String(body.item?.otpauth_url || ""));
    setTotpMessage(tUI("consoleCopy.setupPendingScanOtpauthWithYourAuthenticatorThen"));
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
      setTotpMessage(body.error?.message || tUI("consoleCopy.enablingFailed"));
      return;
    }
    form.reset();
    setTotpStatus("enabled");
    setTotpSecret("");
    setTotpURL("");
    setTotpMessage(tUI("consoleCopy.enabledDisableItImmediatelyOnASharedAdministrator"));
  }

  async function disable2FA(code: string): Promise<boolean> {
    try {
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
      setTotpMessage(body.error?.message || tUI("consoleCopy.disablingFailed"));
      return false;
    }
    setTotpCode("");
    setTotpStatus(String(body.status || "disabled"));
    setTotpSecret("");
    setTotpURL("");
    setTotpMessage(tUI("consoleCopy.2faDisabledSensitiveWritesNoLongerRequireTotp"));
    return true;
    } catch {
      setTotpMessage(confirmNetworkUnavailable);
      return false;
    }
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
      setMessage(body.error?.message || tUI("dashboard.needAdmin"));
      return;
    }
    setRate(String(body.thresholds?.success_rate_min ?? 0.5));
    setMinReq(String(body.thresholds?.min_requests ?? 5));
    setPending(String(body.thresholds?.pending_count ?? 1));
    setMessage(tUI("consoleCopy.alertThresholdsLoaded"));
  }
  async function saveThresholds(): Promise<boolean> {
    try {
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
      setMessage(body.error?.message || tUI("user.saveFail"));
      return false;
    }
    setMessage(tUI("consoleCopy.savedSuccessRateAndRequests", { value0: String(body.thresholds?.success_rate_min), value1: String(body.thresholds?.min_requests) }));
    return true;
    } catch {
      setMessage(confirmNetworkUnavailable);
      return false;
    }
}
  return (
    <AdminShell>
      <NotificationAdminSettings />
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 level={1} k="settings" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-4 text-sm text-ink-secondary">{tUI("consoleCopy.administrator2faUsesTotpInterfaceLanguagesChineseEnglish")}</p>
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" onClick={() => setLocale("auto")}>
            {tUI("chrome.localeAuto")}</Button>
          <Button type="button" variant="outline" onClick={() => setLocale("zh")}>
            {tUI("chrome.localeZh")}</Button>
          <Button type="button" variant="outline" onClick={() => setLocale("en")}>
            English
          </Button>
          <Button type="button" variant="outline" onClick={() => setLocale("ja")}>
            {tUI("chrome.localeJa")}</Button>
        </div>
      </section>
      <IfCan action="settings.totp">
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="twofa" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">
          {tUI("consoleCopy.statusReadsNeverReturnSecretsScanWithAn")}</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Button size="sm" variant="outline" onClick={load2FA}>
            {tUI("consoleCopy.read2faStatus")}</Button>
          <Button size="sm" variant="outline" onClick={setup2FA}>
            {tUI("consoleCopy.startSetup")}</Button>
        </div>
        <p className="mb-3 text-sm text-ink-secondary">{tUI("consoleCopy.current")}{totpStatus}</p>
        {totpSecret ? <p className="mb-3 break-all text-xs text-ink-secondary">secret={totpSecret}</p> : null}
        {totpURL ? <p className="mb-3 break-all text-xs text-ink-secondary">{totpURL}</p> : null}
        <form className="mb-3 flex flex-wrap gap-2" onSubmit={enable2FA}>
          <Input name="enable_code" aria-label={tUI("consoleCopy.totpForEnabling")} placeholder={tUI("consoleCopy.sixDigitCodeToEnable")} />
          <Button size="sm" type="submit">
            {tUI("consoleCopy.confirmEnabling")}</Button>
        </form>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input
            name="disable_code"
            aria-label={tUI("consoleCopy.totpForDisabling")}
            placeholder={tUI("consoleCopy.sixDigitCodeToDisableRequiredWhenEnabled")}
            value={totpCode}
            onChange={(e) => setTotpCode(e.target.value)}
          />
          <ConfirmButton size="sm" title={tUI("consoleCopy.confirm2faDisabling")} description={tUI("consoleCopy.sensitiveWritesWillNoLongerRequireTotpAn")} onConfirm={() => disable2FA(totpCode.trim())}>
            {tUI("consoleCopy.disable2fa")}</ConfirmButton>
        </div>
        <p className="text-sm text-ink-secondary">{totpMessage}</p>
      </section>
      </IfCan>
      <IfCan action="settings.thresholds.view">
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="thresholds" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">{tUI("consoleCopy.minimumSuccessRateMinimumRequestsAndReconciliationQueue")}</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-28" value={rate} onChange={(e) => setRate(e.target.value)} aria-label={tUI("consoleCopy.minimumSuccessRate")} />
          <Input className="w-28" value={minReq} onChange={(e) => setMinReq(e.target.value)} aria-label={tUI("consoleCopy.minimumRequests")} />
          <Input className="w-28" value={pending} onChange={(e) => setPending(e.target.value)} aria-label={tUI("consoleCopy.reconciliationQueueSize")} />
          <Button type="button" variant="outline" onClick={loadThresholds}>
            {tUI("consoleCopy.readThresholds")}</Button>
          <IfCan action="settings.thresholds">
          <ConfirmButton size="sm" variant="outline" title={tUI("consoleCopy.confirmThresholdChanges")} description={tUI("consoleCopy.theseThresholdsAreUsedWhenEvaluatingAlerts")} onConfirm={saveThresholds}>
            {tUI("consoleCopy.saveThresholds")}</ConfirmButton>
          </IfCan>
        </div>
        <p className="text-sm text-ink-secondary">{message}</p>
      </section>
      </IfCan>
      <IfRoles roles={["platform_admin", "ops_admin", "tech_admin"]}>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="ops" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">{tUI("consoleCopy.healthProbesIncurNoChargesAnOpenCircuit")}</p>
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
              setMessage(res.ok ? tUI("consoleCopy.health428", { value0: String(body.health) }) : body.error?.message || tUI("consoleCopy.probeFailed"));
            }}
          >
            {tUI("consoleCopy.healthProbe")}</Button>
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
              setMessage(res.ok ? tUI("consoleCopy.circuitOpened", { value0: String(providerID) }) : body.error?.message || tUI("consoleCopy.couldNotOpenTheCircuit"));
            }}
          >
            {tUI("consoleCopy.openCircuit")}</Button>
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
              setMessage(res.ok ? tUI("consoleCopy.circuitReset", { value0: String(providerID) }) : body.error?.message || tUI("consoleCopy.resetFailed"));
            }}
          >
            {tUI("consoleCopy.resetCircuit")}</Button>
          </IfCan>
        </div>
        <div className="flex flex-wrap gap-2">
          <Input className="w-40" value={canarySlug} onChange={(e) => setCanarySlug(e.target.value)} aria-label={tUI("consoleCopy.canaryProviderSlug")} />
          <Input className="w-24" value={canaryPercent} onChange={(e) => setCanaryPercent(e.target.value)} aria-label={tUI("consoleCopy.canaryPercentage")} />
          <Button
            size="sm"
            variant="outline"
            onClick={async () => {
              const res = await fetch(`${apiBase}/admin/ops/canary`, { credentials: "include" });
              const body = await res.json();
              if (!res.ok) {
                setMessage(body.error?.message || tUI("consoleCopy.couldNotReadCanaryConfiguration"));
                return;
              }
              setCanarySlug(body.canary?.provider_slug || "");
              setCanaryPercent(String(body.canary?.percent ?? 0));
              setMessage(tUI("consoleCopy.canary", { value0: String(body.canary?.provider_slug || "-"), value1: String(body.canary?.percent ?? 0) }));
            }}
          >
            {tUI("consoleCopy.readCanaryConfiguration")}</Button>
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
              setMessage(res.ok ? tUI("consoleCopy.canaryConfigured", { value0: String(body.canary?.provider_slug), value1: String(body.canary?.percent) }) : body.error?.message || tUI("consoleCopy.configurationFailed"));
            }}
          >
            {tUI("consoleCopy.saveCanaryConfiguration")}</Button>
        </div>
      </section>
      </IfRoles>
      <IfCan action="settings.backup">
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="backup" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">{tUI("consoleCopy.checksPostgresRedisAndMigrationsAndRecordsRpo")}</p>
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
                ? tUI("consoleCopy.drillRpoMinutesRtoMinutes", { value0: String(body.item?.status || "ok"), value1: String(body.item?.rpo_minutes), value2: String(body.item?.rto_minutes) })
                : body.error?.message || tUI("consoleCopy.drillFailed"),
            );
          }}
        >
          {tUI("adminUi.backup")}</Button>
        <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      </section>
      </IfCan>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="drill" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">
          {tUI("consoleCopy.paymentDrillsMustRejectForgedSignaturesMediaDrills")}</p>
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
              setDrillMessage(res.ok ? tUI("consoleCopy.paymentDrill", { value0: String(body.item?.status), value1: String(body.item?.detail) }) : body.error?.message || tUI("consoleCopy.paymentDrillFailed"));
            }}
          >
            {tUI("consoleCopy.paymentDrill450")}</Button>
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
              setDrillMessage(res.ok ? tUI("consoleCopy.mediaDrill", { value0: String(body.item?.status), value1: String(body.item?.detail) }) : body.error?.message || tUI("consoleCopy.mediaDrillFailed"));
            }}
          >
            {tUI("consoleCopy.mediaDrill453")}</Button>
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
              setDrillMessage(res.ok ? tUI("consoleCopy.tlsDrill", { value0: String(body.item?.status), value1: String(body.item?.detail) }) : body.error?.message || tUI("consoleCopy.tlsDrillFailed"));
            }}
          >
            {tUI("consoleCopy.tlsDrill456")}</Button>
          </IfCan>
          <p className="text-sm text-ink-secondary">{drillMessage}</p>
        </div>
      </section>
      <section className="rounded-card border border-hairline bg-canvas-raised  p-6">
        <AdminH2 k="oem" className="mb-4 text-lg font-semibold tracking-tight" />
        <p className="mb-3 text-sm text-ink-secondary">{tUI("consoleCopy.localhostAndEmptyDomainsUseSandboxIssuanceWith")}</p>
        <div className="mb-3 flex flex-wrap gap-2">
          <Input className="w-56" value={brandID} onChange={(e) => setBrandID(e.target.value)} aria-label={tUI("consoleCopy.brandId")} placeholder="brd_oem" />
          <Button
            size="sm"
            variant="outline"
            onClick={async () => {
              const res = await fetch(`${apiBase}/admin/brands`, { credentials: "include" });
              const body = await res.json();
              if (!res.ok) {
                setMessage(body.error?.message || tUI("consoleCopy.couldNotLoadTheBrand"));
                return;
              }
              const items = body.items || [];
              const hit = items.find((item: { id?: string }) => item.id === brandID) || items[0];
              setMessage(hit ? `${hit.id} CNAME=${hit.cname_target || "-"} TLS=${hit.tls_status || "pending"} issuer=${hit.tls_issuer || "sandbox"}` : tUI("consoleCopy.noBrand"));
            }}
          >
            {tUI("consoleCopy.readBrand")}</Button>
          <IfCan action="settings.drill.tls">
          <ConfirmButton
            size="sm"
            title={tUI("consoleCopy.confirmCertificateIssuance")}
            description={tUI("consoleCopy.sandboxDomainsAreOnlyMarkedIssuedPublicDomains")}
            onConfirm={async () => {
                    try {
              const res = await fetch(`${apiBase}/admin/brands/${brandID}/tls/issue`, {
                method: "POST",
                credentials: "include",
                headers: confirmHeaders,
                body: "{}",
              });
              const body = await res.json();
              setMessage(
                res.ok
                  ? tUI("consoleCopy.issued", { value0: String(body.item?.id), value1: String(body.item?.tls_status), value2: String(body.item?.cname_target), value3: String(body.item?.tls_issuer || "sandbox") })
                  : body.error?.message || tUI("consoleCopy.issuanceFailed"),
              );
                    return true;
                    } catch {
                      setMessage(confirmNetworkUnavailable);
                      return false;
                    }
}}
          >
            {tUI("consoleCopy.issueCertificate")}</ConfirmButton>
          </IfCan>
        </div>
        <p className="text-sm text-ink-secondary">{message}</p>
      </section>
    </AdminShell>
  );
}
