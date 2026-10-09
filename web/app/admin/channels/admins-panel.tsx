"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "next-intl";
import { useViewer } from "@/components/rbac/viewer-context";
import { ConfirmButton } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiClient } from "@/lib/client";
import { confirmHeaders } from "@/lib/confirm";

type Member = { user_id: string; email: string };
export function ChannelAdminsPanel({ channelID, code }: { channelID: string; code: string }) {
  const t = useTranslations("oemDelivery");
  const viewer = useViewer();
  const [email, setEmail] = useState("");
  const [reason, setReason] = useState("");
  const [search, setSearch] = useState("");
  const [submittedSearch, setSubmittedSearch] = useState("");
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const path = `/admin/channels/${encodeURIComponent(channelID)}/admins`;
  async function read(href: string) {
    const body = await apiClient<{ items?: Member[]; error?: { message: string } }>("GET", href);
    if (body.error || !body.items) throw new Error(body.error?.message || t("loadFailed"));
    return body.items;
  }
  const query = useQuery({ queryKey: [viewer.userId, path], queryFn: () => read(path) });
  const candidates = useQuery({ queryKey: [viewer.userId, path, "candidates", submittedSearch], queryFn: () => read(`${path}/candidates?q=${encodeURIComponent(submittedSearch)}`) });
  async function submit(enabled: boolean) {
    setError(""); setMessage("");
    try {
      const body = await apiClient<{ changed?: boolean; error?: { message: string } }>("POST", path, { headers: confirmHeaders, body: JSON.stringify({ email, reason: reason.trim(), enabled }) });
      if (body.error) { setError(body.error.message); setMessage(body.error.message); return false; }
      const result = await query.refetch();
      await candidates.refetch();
      setMessage(result.isError ? t("savedReadFailed") : t("adminSaved"));
      return true;
    } catch (caught) { const message = caught instanceof Error && "status" in caught && Number(caught.status) >= 400 && Number(caught.status) < 500 ? caught.message : t("resultUnknown"); setError(message); setMessage(message); return false; }
  }
  return <section className="rounded-card border border-hairline bg-canvas-raised p-6">
    <h2 className="text-lg font-semibold">{t("adminsTitle")}</h2><p className="mt-2 text-sm text-ink-secondary">{t("adminsHint")}</p>
    {query.isPending ? <p role="status">{t("loading")}</p> : query.isError ? <p role="alert">{t("loadFailed")}</p> : <ul className="my-3">{query.data?.length ? query.data.map(member => <li key={member.user_id}>{member.email}</li>) : <li>{t("noAdmins")}</li>}</ul>}
    <form className="my-3 flex gap-2" onSubmit={event => { event.preventDefault(); setSubmittedSearch(search.trim()); }}><Input aria-label={t("searchUser")} placeholder={t("searchUser")} value={search} onChange={event => setSearch(event.target.value)} /><Button variant="outline" type="submit">{t("refresh")}</Button></form>
    {candidates.isError ? <p role="alert">{t("loadFailed")}</p> : candidates.data?.length === 0 ? <p>{t("noCandidates")}</p> : null}
    <div className="my-3 grid gap-3"><select aria-label={t("receiver")} className="h-10 rounded-control border border-hairline bg-canvas-raised px-3" value={email} onChange={event => setEmail(event.target.value)}><option value="">{t("receiver")}</option>{candidates.data?.map(member => <option key={member.user_id} value={member.email}>{member.email}</option>)}</select><Input aria-label={t("optionalReason")} placeholder={t("optionalReason")} value={reason} onChange={event => setReason(event.target.value)} /></div>
    <div className="flex flex-wrap gap-3">{[true, false].map(enabled => <ConfirmButton key={String(enabled)} variant={enabled ? "default" : "outline"} disabled={!email || query.isPending || query.isError || candidates.isPending || candidates.isError} title={t(enabled ? "grant" : "revoke")} description={t("adminConfirm", { name: code, email, action: t(enabled ? "grant" : "revoke") })} error={error} onConfirm={() => submit(enabled)}>{t(enabled ? "grant" : "revoke")}</ConfirmButton>)}</div>
    {message ? <p role="status" className="mt-3 text-sm">{message}</p> : null}
  </section>;
}
