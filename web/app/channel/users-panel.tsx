"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { ConfirmButton } from "@/components/confirm-button";
import { LedgerTable } from "@/components/console/ledger-table";
import { ListResourceView } from "@/components/console/list-resource-view";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card, CardTitle } from "@/components/ui/card";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { fetchListItems } from "@/lib/list-resource";

type ChannelUser = { id?: string; email?: string; status?: string; source_code?: string; roles?: string[] };

export default function ChannelUsers({ channelID, managedChannelID }: { channelID?: string; managedChannelID?: string }) {
  const t = useTranslations("channelUi");
  const [selected, setSelected] = useState<ChannelUser | null>(null);
  const [reason, setReason] = useState("");
  const [message, setMessage] = useState("");
  const list = useListResource<ChannelUser>({
    queryKey: channelID || managedChannelID || "",
    load: () => fetchListItems(`${apiBase}${channelID ? `/channel/subchannels/${encodeURIComponent(channelID)}/users` : `/channel/users${managedChannelID ? `?channel_id=${encodeURIComponent(managedChannelID)}` : ""}`}`),
  });

  async function changeStatus(): Promise<boolean> {
    if (!selected?.id || !reason.trim()) return false;
    try {
      const action = selected.status === "banned" ? "unban" : "ban";
      const response = await fetch(`${apiBase}/channel/users/${encodeURIComponent(selected.id)}/${action}`, {
        method: "POST", credentials: "include", headers: confirmHeaders,
        body: JSON.stringify({ reason: reason.trim() }),
      });
      const body = await response.json();
      if (!response.ok) {
        setMessage(body.error?.message || "操作失败");
        return false;
      }
      setMessage(`已${action === "ban" ? "封禁" : "解封"} ${selected.email}`);
      setSelected(null);
      setReason("");
      await list.reload();
      return true;
    } catch {
      setMessage(confirmNetworkUnavailable);
      return false;
    }
  }

  return (
    <Card>
      <CardTitle className="mb-4 text-lg font-semibold tracking-tight">{t("usersTitle")}</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">{t("usersLead")}</p>
      <Button variant="outline" onClick={() => void list.reload()}>
        {t("refreshUsers")}
      </Button>
      {message ? <p role="status" className="mt-2 text-sm text-ink-secondary">{message}</p> : null}
      <ListResourceView
        snapshot={list.snapshot}
        loadingTitle={t("usersTitle")}
        emptyTitle={t("emptyUsers")}
        emptyDetail={t("emptyUsersDetail")}
        onRetry={() => void list.reload()}
      >
        <LedgerTable
          columns={[t("colEmail"), t("colStatus"), t("colCode"), "操作"]}
          emptyTitle={t("emptyUsers")}
          emptyDetail={t("emptyUsersDetail")}
          rows={list.snapshot.items.map((item) => ({
            key: item.id || `${item.email}-${item.source_code}`,
            cells: [item.email || "—", item.status || "—", item.source_code || "—", <Button key="action" size="sm" variant="outline" disabled={item.roles?.some((role) => role !== "end_user") || !item.id} onClick={() => { setSelected(item); setReason(""); setMessage(""); }}>{item.status === "banned" ? "解封" : "封禁"}</Button>],
          }))}
        />
      </ListResourceView>
      {selected ? <section className="mt-4 grid max-w-lg gap-3 rounded-control border border-hairline p-4">
        <p>{selected.status === "banned" ? "解封" : "封禁"} · {selected.email}</p>
        <Input aria-label="用户操作原因" placeholder="填写操作原因" value={reason} onChange={(event) => setReason(event.target.value)} />
        <div className="flex gap-2">
          <ConfirmButton size="sm" disabled={!reason.trim()} title={`确认${selected.status === "banned" ? "解封" : "封禁"}用户`} description={`${selected.email}；原因：${reason.trim()}。`} onConfirm={changeStatus}>{selected.status === "banned" ? "解封" : "封禁"}</ConfirmButton>
          <Button size="sm" variant="outline" onClick={() => setSelected(null)}>取消</Button>
        </div>
      </section> : null}
    </Card>
  );
}
