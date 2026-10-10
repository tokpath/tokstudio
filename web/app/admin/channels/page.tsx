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

type Channel = { id: string; code: string; type: string; status: string; brand_id: string; brand_name?:string; parent_id?: string };
export default function AdminChannelsPage() {
  const t = useTranslations("oemDelivery");
  const tw = useTranslations("channelWorkbench");
  const tc = useTranslations("common");
  const [kind, setKind] = useState<"C" | "B">("C");
  const [createChannel, setCreateChannel] = useState(false);
  const [createOEM, setCreateOEM] = useState(false);
  const model = useSearchParams().get("model");
  return <AdminShell>
    <section className="rounded-card border border-hairline bg-canvas-raised p-6">
      <h1 className="text-2xl font-semibold">{t("organizations")}</h1>
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
        { accessorKey: "code", header: tw("organization") },
        { accessorKey: "brand_name", header: t("brand"), cell:({row})=><span>{row.original.brand_name||row.original.brand_id}</span> },
        { accessorKey: "status", header: tw("status"), cell:({row})=>row.original.status==="active"?tc("stActive"):row.original.status==="disabled"?tc("stDisabled"):row.original.status },
      ]}
    />
    <OEMCreateDialog open={createOEM} onOpenChange={setCreateOEM} />
    <CreateChannelDialog open={createChannel} onOpenChange={setCreateChannel} />
  </AdminShell>;
}
