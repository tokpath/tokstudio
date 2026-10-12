"use client";

import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { apiClient } from "@/lib/client";
import { confirmHeaders } from "@/lib/confirm";
import { useViewer } from "@/components/rbac/viewer-context";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ConfirmButton } from "@/components/confirm-button";

type GoogleConfig = {
  client_id: string;
  secret_set: boolean;
  enabled: boolean;
  version: number;
  redirect_uris: string[];
  verification: Record<string, { verified_at?: string; error?: string }>;
};

export function OEMGooglePanel({ channelID }: { channelID: string }) {
  const t = useTranslations("oemGoogle");
  const viewer = useViewer();
  const cache = useQueryClient();
  const path = `/admin/oem-deliveries/${encodeURIComponent(channelID)}/google`;
  const query = useQuery({
    queryKey: [viewer.userId, path],
    refetchOnWindowFocus: false,
    queryFn: async () => {
      const body = await apiClient<{ item: GoogleConfig }>("GET", path);
      return body.item;
    },
  });
  const [clientID, setClientID] = useState("");
  const [secret, setSecret] = useState("");
  const [enabled, setEnabled] = useState(false);
  const [version, setVersion] = useState(0);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  const [testLink, setTestLink] = useState("");
  useEffect(() => {
    if (query.data) {
      setClientID(query.data.client_id);
      setEnabled(query.data.enabled);
      setVersion(query.data.version);
      setSecret("");
      setTestLink("");
    }
  }, [query.data]);

  const changedCredentials = clientID.trim() !== query.data?.client_id || secret.trim() !== "";
  const dirty = changedCredentials || enabled !== query.data?.enabled;
  const verifiedPrimary = Boolean(query.data?.verification[query.data.redirect_uris[0]]?.verified_at);

  async function save() {
    setBusy(true); setError(""); setMessage("");
    try {
      const body = await apiClient<{ item: GoogleConfig }>("PUT", path, {
        headers: { "Content-Type": "application/json", ...confirmHeaders },
        body: JSON.stringify({ client_id: clientID.trim(), client_secret: secret.trim(), enabled, expected_version: version }),
      });
      cache.setQueryData([viewer.userId, path], body.item);
      setSecret("");
      setVersion(body.item.version);
      setMessage(t("saved"));
      return true;
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : t("failed"));
      return false;
    } finally { setBusy(false); }
  }

  async function test(redirectURI: string) {
    const popup = window.open("about:blank", "_blank");
    if (popup) popup.opener = null;
    setBusy(true); setError(""); setMessage(""); setTestLink("");
    try {
      const body = await apiClient<{ url: string }>("POST", `${path}/test`, {
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ redirect_uri: redirectURI }),
      });
      if (popup) popup.location.href = body.url;
      else setTestLink(body.url);
      setMessage(t("testStarted"));
    } catch (caught) {
      popup?.close();
      setError(caught instanceof Error ? caught.message : t("failed"));
    } finally { setBusy(false); }
  }

  return <section id="google" aria-label={t("title")} className="rounded-card border border-hairline bg-canvas-raised p-6">
    <div className="flex flex-wrap items-center justify-between gap-3">
      <h2 className="font-semibold">{t("title")}</h2>
      <Button variant="outline" disabled={busy} onClick={() => void query.refetch()}>{t("refresh")}</Button>
    </div>
    <p className="mt-2 text-sm text-ink-secondary">{t("intro")}</p>
    {query.isPending ? <p role="status" className="mt-4">{t("loading")}</p> : query.isError || !query.data ? <p role="alert" className="mt-4 text-danger">{t("failed")}</p> : <>
      <div className="my-5 grid gap-4 md:grid-cols-2">
        <label className="grid gap-1 text-sm">Google Client ID
          <Input autoComplete="off" value={clientID} disabled={busy} onChange={event => { setClientID(event.target.value); setEnabled(false); }} />
        </label>
        <label className="grid gap-1 text-sm">Google Client Secret
          <Input type="password" autoComplete="new-password" value={secret} disabled={busy} placeholder={query.data.secret_set ? t("keepSecret") : t("enterSecret")} onChange={event => { setSecret(event.target.value); if (event.target.value) setEnabled(false); }} />
        </label>
      </div>
      <label className="flex items-center gap-2 text-sm"><input type="checkbox" checked={enabled} disabled={busy || changedCredentials || (!enabled && !verifiedPrimary)} onChange={event => setEnabled(event.target.checked)} />{t("enabled")}</label>
      {!verifiedPrimary || changedCredentials ? <p className="mt-2 text-sm text-ink-secondary">{t("enableHint")}</p> : null}
      <div className="my-5 divide-y divide-hairline">
        {query.data.redirect_uris.map((uri, index) => {
          const proof = query.data.verification[uri];
          return <div key={uri} className="space-y-2 py-3">
            <p className="text-sm font-medium">{t(index === 0 ? "primaryCallback" : "adminCallback")}</p>
            <p className="break-all font-mono text-sm">{uri}</p>
            <p className={`text-sm ${proof?.error ? "text-danger" : "text-ink-secondary"}`}>{proof?.verified_at ? t("verified", { time: new Date(proof.verified_at).toLocaleString(undefined, { timeZone: "Asia/Shanghai" }) }) : proof?.error ? t("testFailed", { reason: proof.error }) : t("unverified")}</p>
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" onClick={() => void navigator.clipboard.writeText(uri).then(() => setMessage(t("copied"))).catch(() => setError(t("copyFailed")))}>{t("copy")}</Button>
              <Button variant="outline" disabled={busy || dirty || !query.data?.secret_set} onClick={() => void test(uri)}>{t("test")}</Button>
            </div>
          </div>;
        })}
      </div>
      <p className="mb-4 text-sm text-ink-secondary">{t("callbacksHint")}</p>
      <ConfirmButton disabled={busy || !dirty} title={t("save")} description={t("confirm")} error={error} validate={() => { const valid = clientID.trim().endsWith(".apps.googleusercontent.com") && (clientID.trim() === query.data?.client_id || secret.trim() !== ""); if (!valid) setError(t("invalid")); return valid; }} onConfirm={save}>{t("save")}</ConfirmButton>
    </>}
    {testLink ? <a className="mt-3 block text-sm text-brand-emphasis underline" target="_blank" rel="noopener noreferrer" href={testLink}>{t("continueTest")}</a> : null}
    {error ? <p className="mt-3 text-sm text-danger" role="alert">{error}</p> : null}
    {message ? <p className="mt-3 text-sm" role="status">{message}</p> : null}
  </section>;
}
