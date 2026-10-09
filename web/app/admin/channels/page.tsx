"use client";

import { useState } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { AdminListPanel } from "../list-panel";
import { AdminShell } from "../shell";
import { CreateChannelDialog, OpenCreateButton } from "./create-dialogs";
import { OEMCreateDialog } from "./oem-create-dialog";
import { Button } from "@/components/ui/button";
import { IfCan } from "@/components/rbac/if-can";
import { modelEditHref } from "@/lib/catalog";

type Channel = { id: string; code: string; type: string; status: string; brand_id: string; parent_id?: string };
export default function AdminChannelsPage() {
  const t = useTranslations("oemDelivery");
  const [kind, setKind] = useState<"C" | "B">("C");
  const [createChannel, setCreateChannel] = useState(false);
  const [createOEM, setCreateOEM] = useState(false);
  const model = useSearchParams().get("model");
  return <AdminShell>
    <section className="rounded-card border border-hairline bg-canvas-raised p-6">
      <h2 className="text-lg font-semibold">{t("organizations")}</h2>
      <p className="mt-2 text-sm text-ink-secondary">{t("organizationsHint")}</p>
      {model ? <Link className="mt-3 block text-sm text-brand-emphasis underline" href={modelEditHref(model)}>{model}</Link> : null}
    </section>
    <div className="flex gap-2" role="tablist" aria-label={t("organizations")}>
      {(["C", "B"] as const).map(type => <Button key={type} role="tab" aria-selected={kind === type} variant={kind === type ? "default" : "outline"} onClick={() => setKind(type)}>{t(type === "C" ? "oemList" : "channelList")}</Button>)}
    </div>
    <AdminListPanel<Channel>
      key={kind}
      path={`/admin/channels?managed=1&type=${kind}`}
      title={t(kind === "C" ? "oemList" : "channelList")}
      rowHref={row => `${row.type === "C" ? "/admin/oem-deliveries" : "/admin/channels"}/${encodeURIComponent(row.id)}${model ? `?model=${encodeURIComponent(model)}` : ""}`}
      emptyTitle={t(kind === "C" ? "noOEM" : "noChannel")}
      actions={<IfCan action="channels.write"><OpenCreateButton label={t(kind === "C" ? "createTitle" : "newChannel")} onClick={() => kind === "C" ? setCreateOEM(true) : setCreateChannel(true)} /></IfCan>}
      columns={[
        { accessorKey: "code", header: t("name") },
        { accessorKey: "brand_id", header: t("brand") },
        { accessorKey: "status", header: t("check.organization") },
      ]}
    />
    <OEMCreateDialog open={createOEM} onOpenChange={setCreateOEM} />
    <CreateChannelDialog open={createChannel} onOpenChange={setCreateChannel} />
  </AdminShell>;
}
