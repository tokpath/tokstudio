"use client";

import { networkUnavailable } from "@/lib/confirm";
import { useTranslations } from "next-intl";
import { useViewer } from "@/components/rbac/viewer-context";
import { canChannelAction, canWrite } from "@/lib/rbac";

import { useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ConfirmButton } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { EmptyState } from "@/components/empty-state";
import { apiBase } from "@/lib/api";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { CreditCard } from "lucide-react";

type Field = { key: string; label: string; secret?: boolean; required?: boolean };
type Lane = {
  adapter: string;
  display_name: string;
  brand_color?: string;
  state: string;
  instance_count: number;
  missing_fields?: string[];
  schema?: Field[];
  auto_renew_supported?: boolean;
  checkout_mode?: string;
  pay_currency?: string;
};
type Overview = {
  item?: { lanes?: Lane[]; callback_origin?: string; online_disabled?: boolean; issue_ratio_bps?: number; fen_per_usd?: number };
  channel_type?: string;
  hint?: string;
};

const stateTone: Record<string, "neutral" | "brand" | "success" | "warn"> = {
  none: "neutral",
  configuring: "warn",
  sandbox: "brand",
  live: "success",
  disabled: "warn",
};

export function PaymentLanesPanel({ scope = "channel" }: { scope?: "channel" | "admin" }) {
  const tUI = useTranslations();
  const confirmNetworkUnavailable = networkUnavailable(tUI);
  const basePath = `/${scope}/payments`;
  const permissionViewer = useViewer();
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: [`${basePath}/overview`],
    queryFn: async () => {
      const res = await fetch(`${apiBase}${basePath}/overview`, { credentials: "include" });
      const body = (await res.json()) as Overview & { error?: { message?: string } };
      if (!res.ok) {
        throw new Error(body.error?.message || tUI("consoleCopy.loadingFailed", { value0: String(res.status) }));
      }
      return body;
    },
  });
  const [open, setOpen] = useState<Lane | null>(null);
  const [step, setStep] = useState(0);
  const [name, setName] = useState("");
  const [mode, setMode] = useState("sandbox");
  const [creds, setCreds] = useState<Record<string, string>>({});
  const [message, setMessage] = useState("");
  const [instanceID, setInstanceID] = useState("");

  const lanes = query.data?.item?.lanes || [];
  const origin = query.data?.item?.callback_origin || "";
  const fields = open?.schema || [];
  const webhookPath = open ? `/v1/payments/${open.adapter}/webhook` : "";

  const stateLabel = useMemo(
    () =>
      ({
        none: tUI("consoleCopy.notActivated"),
        configuring: tUI("oemDelivery.phase.configuring"),
        sandbox: tUI("consoleCopy.sandboxAvailable"),
        live: tUI("consoleCopy.activated"),
        disabled: tUI("common.stDisabled"),
      }) as Record<string, string>,
    [],
  );

  function resetWizard(lane: Lane) {
    setOpen(lane);
    setStep(0);
    setName(tUI("consoleCopy.merchant", { value0: String(lane.display_name) }));
    setMode("sandbox");
    setCreds({});
    setInstanceID("");
    setMessage("");
  }

  async function createDraft(): Promise<string> {
    const res = await fetch(`${apiBase}${basePath}/instances`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ adapter: open?.adapter, name, mode }),
    });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || tUI("common.createFailed"));
      return "";
    }
    const id = String(body.item?.id || "");
    setInstanceID(id);
    return id;
  }

  async function saveCredentials() {
    const id = instanceID || (await createDraft());
    if (!id) return false;
    const res = await fetch(`${apiBase}${basePath}/instances/${id}`, {
      method: "PATCH",
      credentials: "include",
      headers: confirmHeaders,
      body: JSON.stringify({ credentials: creds, name, mode }),
    });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || tUI("consoleCopy.couldNotSaveCredentials"));
      return false;
    }
    setMessage(tUI("consoleCopy.credentialsSavedSecretsWillNotBeShownAgain"));
    return true;
  }

  async function testConn() {
    const res = await fetch(`${apiBase}${basePath}/instances/${instanceID}/test`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: "{}",
    });
    const body = await res.json();
    if (!res.ok) {
      setMessage(body.error?.message || tUI("consoleCopy.connectionFailedCheckTheRequiredFields"));
      return false;
    }
    setMessage(tUI("consoleCopy.connectionSuccessful"));
    return true;
  }

  if (query.isPending) {
    return <p className="text-sm text-ink-secondary">{tUI("consoleCopy.loadingPaymentMethods")}</p>;
  }
  if (query.isError) {
    const detail = query.error instanceof Error ? query.error.message : tUI("common.listFailed");
    return (
      <EmptyState
        icon={CreditCard}
        title={tUI("consoleCopy.couldNotLoadPaymentMethods")}
        detail={tUI("consoleCopy.refreshAndTryAgain", { value0: String(detail) })}
      />
    );
  }
  if (lanes.length === 0) {
    return (
      <EmptyState
        icon={CreditCard}
        title={tUI("consoleCopy.noPaymentPluginsYet")}
        detail={tUI("consoleCopy.noAvailablePaymentMethodsAreConfigured")}
      />
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-ink-secondary">{query.data?.hint}</p>
      <div className="grid gap-4 md:grid-cols-3">
        {lanes.map((lane) => (
          <section key={lane.adapter} className="rounded-card border border-hairline bg-canvas-raised p-5">
            <div className="mb-3 flex items-center justify-between gap-2">
              <h2 className="text-lg font-medium" style={{ color: lane.brand_color }}>
                {lane.display_name}
              </h2>
              <Badge tone={stateTone[lane.state] || "neutral"}>{stateLabel[lane.state] || lane.state}</Badge>
            </div>
            <p className="mb-4 text-sm text-ink-secondary">
              {lane.state === "none"
                ? tUI("consoleCopy.afterActivationThisChannelSTopUpPage", { value0: String(lane.display_name) })
                : lane.missing_fields?.length
                  ? tUI("consoleCopy.stillRequired", { value0: String(lane.missing_fields.join("、")) })
                  : tUI("consoleCopy.merchantsConfiguredSettlementCurrency", { value0: String(lane.instance_count), value1: String(lane.pay_currency) })}
            </p>
            <Button size="sm" disabled={!(scope === "admin" ? canWrite("payments.write", permissionViewer) : canChannelAction("paymentSettings", permissionViewer))} onClick={() => resetWizard(lane)}>
              {lane.state === "none" ? tUI("consoleCopy.activate", { value0: String(lane.display_name) }) : tUI("consoleCopy.manageCredentials")}
            </Button>
          </section>
        ))}
      </div>
      <Dialog open={!!open} onOpenChange={(next) => !next && setOpen(null)}>
        <DialogContent className="max-w-xl">
          <DialogHeader>
            <DialogTitle>{tUI("consoleCopy.activate517")}{open?.display_name}</DialogTitle>
            <DialogDescription>
              {tUI("consoleCopy.step")}{step + 1} {tUI("consoleCopy.5CredentialFieldsFollowEachPaymentMethodS")}</DialogDescription>
          </DialogHeader>
          {step === 0 ? (
            <div className="grid gap-3">
              <Label htmlFor="ppi-name">{tUI("consoleCopy.internalName")}</Label>
              <Input id="ppi-name" value={name} onChange={(e) => setName(e.target.value)} />
              <Label htmlFor="ppi-mode">{tUI("consoleCopy.environment")}</Label>
              <select
                id="ppi-mode"
                className="h-10 rounded-control border border-hairline bg-canvas-raised px-3 text-sm"
                value={mode}
                onChange={(e) => setMode(e.target.value)}
              >
                <option value="sandbox">{tUI("consoleCopy.sandbox")}</option>
                <option value="live">{tUI("consoleCopy.production")}</option>
              </select>
            </div>
          ) : null}
          {step === 1 ? (
            <div className="grid max-h-80 gap-3 overflow-y-auto">
              {fields.map((field) => (
                <div key={field.key} className="grid gap-1">
                  <Label htmlFor={field.key}>
                    {field.label}
                    {field.required ? " *" : ""}
                  </Label>
                  <Input
                    id={field.key}
                    type={field.secret ? "password" : "text"}
                    placeholder={field.secret ? tUI("consoleCopy.leaveBlankToRetainTheSavedValue") : ""}
                    value={creds[field.key] || ""}
                    onChange={(e) => setCreds((prev) => ({ ...prev, [field.key]: e.target.value }))}
                  />
                </div>
              ))}
            </div>
          ) : null}
          {step === 2 ? (
            <div className="grid gap-2 text-sm">
              <p className="text-ink-secondary">{tUI("consoleCopy.theCallbackIsReadOnlyTheSystemIdentifies")}</p>
              <p className="th-eyebrow text-ink-mute">ORIGIN</p>
              <code className="rounded-stamp bg-canvas px-3 py-2">{origin || "—"}</code>
              <p className="th-eyebrow text-ink-mute">PATH</p>
              <code className="rounded-stamp bg-canvas px-3 py-2">{webhookPath}</code>
              <Button
                size="sm"
                variant="outline"
                onClick={() => void navigator.clipboard.writeText(`${origin}${webhookPath}`)}
              >
                {tUI("consoleCopy.copyFullUrl")}</Button>
            </div>
          ) : null}
          {step === 3 ? (
            <p className="text-sm text-ink-secondary">{tUI("consoleCopy.limitsAreOptionalDefaultsComeFromThePayment")}</p>
          ) : null}
          {step === 4 ? (
            <p className="text-sm text-ink-secondary">{tUI("consoleCopy.testConnectivityFirstAFailedTestBlocksProduction")}</p>
          ) : null}
          {message ? <p className="text-sm text-ink-secondary">{message}</p> : null}
          <DialogFooter>
            {step > 0 ? (
              <Button variant="outline" onClick={() => setStep((n) => n - 1)}>
                {tUI("consoleCopy.previousStep")}</Button>
            ) : null}
            {step === 1 ? (
              <ConfirmButton title={tUI("consoleCopy.saveCredentials")} description={tUI("consoleCopy.secretsAreStoredEncryptedAndNeverReturnedIn")} onConfirm={saveCredentials}>
                {tUI("consoleCopy.saveCredentials")}</ConfirmButton>
            ) : null}
            {step === 4 ? (
              <>
                <Button variant="outline" onClick={() => void testConn()}>
                  {tUI("consoleCopy.testConnection")}</Button>
                {mode === "live" ? (
                  <ConfirmButton
                    title={tUI("consoleCopy.activateProduction")}
                    description={tUI("consoleCopy.productionPaymentsAppearOnTheUserSTop")}
                    onConfirm={async () => {
                    try {
                      const ok = await testConn();
                      if (!ok || !instanceID) return false;
                      const res = await fetch(`${apiBase}${basePath}/instances/${instanceID}/go-live`, {
                        method: "POST",
                        credentials: "include",
                        headers: confirmHeaders,
                        body: "{}",
                      });
                      const body = await res.json();
                      setMessage(res.ok ? tUI("consoleCopy.live") : body.error?.message || tUI("consoleCopy.activationFailed"));
                    const __ok = res.ok;
                      if (res.ok) {
                        setOpen(null);
                        await queryClient.invalidateQueries({ queryKey: [`${basePath}/overview`] });
                      }
                    return __ok;
                    } catch {
                      setMessage(confirmNetworkUnavailable);
                      return false;
                    }
}}
                  >
                    {tUI("consoleCopy.goLive")}</ConfirmButton>
                ) : (
                  <Button
                    onClick={async () => {
                      const ok = await testConn();
                      if (ok) {
                        setOpen(null);
                        await queryClient.invalidateQueries({ queryKey: [`${basePath}/overview`] });
                      }
                    }}
                  >
                    {tUI("consoleCopy.saveSandbox")}</Button>
                )}
              </>
            ) : (
              <Button
                onClick={async () => {
                  if (step === 0 && !instanceID) {
                    const id = await createDraft();
                    if (!id) return;
                  }
                  setStep((n) => Math.min(4, n + 1));
                }}
              >
                {tUI("vibeUi.statNext")}</Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
