"use client";

import { useViewer } from "@/components/rbac/viewer-context";
import { canChannelAction } from "@/lib/rbac";
import { useSearchParams } from "next/navigation";
import { customerReturnHref } from "@/lib/customer";
import { useBrand } from "@/components/brand-context";
import { ProfessionalCustomerList } from "@/components/professional-customer-list";
import { ChannelOnboardingPanel } from "@/components/channel-onboarding";
import { use, useState } from "react";
import Link from "next/link";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ChannelModelsPanel } from "@/app/admin/channels/models-panel";
import { ChannelAdminsPanel } from "@/app/admin/channels/admins-panel";
import ChannelUsers from "@/app/channel/users-panel";
import { ConfirmButton } from "@/components/confirm-button";
import { apiBase } from "@/lib/api";
import { apiClient } from "@/lib/client";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";

type Channel = { id: string; code: string; type: string; status: string; parent_id?: string; brand_id: string };

export default function SubchannelDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const search=useSearchParams();
  const viewer = useViewer();
  const brand=useBrand();const scope=`${viewer.userId || ""}:${brand?.id || ""}`;
  const queryClient = useQueryClient();
  const [message, setMessage] = useState("");
  const query = useQuery({ queryKey: ["/admin/channels", id,scope,viewer.roles.join(",")], enabled:!viewer.loading, queryFn: () => apiClient<{ item: Channel }>("GET", `/admin/channels/${encodeURIComponent(id)}`) });
  const me = useQuery({ queryKey: ["/channel/me",scope,viewer.roles.join(",")],enabled:!viewer.loading, queryFn: () => apiClient<{ channel_org_id: string; channel_type: string }>("GET", "/channel/me") });
  const item = query.data?.item;
  const allowed = item?.type === "B" && item.parent_id === me.data?.channel_org_id && me.data?.channel_type === "C";

  async function changeStatus(): Promise<boolean> {
    if (!item) return false;
    const status = item.status === "active" ? "disabled" : "active";
    try {
      const response = await fetch(`${apiBase}/admin/channels/${encodeURIComponent(id)}`, {
        method: "PATCH", credentials: "include", headers: confirmHeaders,
        body: JSON.stringify({ status }),
      });
      const body = await response.json();
      setMessage(response.ok ? `已${status === "active" ? "启用" : "停用"} ${item.code}` : body.error?.message || "保存失败");
      if (response.ok) await queryClient.invalidateQueries({ queryKey: ["/admin/channels", id] });
      return response.ok;
    } catch {
      setMessage(confirmNetworkUnavailable);
      return false;
    }
  }

  return <div className="grid gap-6">
    <header><Link className="text-sm text-brand-emphasis hover:underline" href={customerReturnHref(search.get("return_to"),"/channel/subchannels",scope)}>← 下属渠道</Link><div className="mt-2 flex flex-wrap items-center justify-between gap-3"><div><h1 className="text-2xl font-semibold">{item?.code || "渠道详情"}</h1><p className="mt-1 text-sm text-ink-secondary">{item ? `渠道 · ${item.status === "active" ? "运行中" : "已停用"} · 继承本平台品牌` : "正在读取渠道…"}</p></div>{item && allowed && canChannelAction("operations", viewer) ? <ConfirmButton size="sm" variant="outline" title={`确认${item.status === "active" ? "停用" : "启用"}渠道`} description={item.status === "active" ? "停用后该渠道的新模型消费立即停止，余额和历史保留。" : "启用后仍须有有效模型授权才可调用。"} onConfirm={changeStatus}>{item.status === "active" ? "停用渠道" : "启用渠道"}</ConfirmButton> : null}</div>{message ? <p role="status" className="mt-2 text-sm text-ink-secondary">{message}</p> : null}{query.isError ? <p role="alert" className="mt-2">读取渠道失败，请返回列表重试。</p> : null}</header>
    {item && me.data && !allowed ? <p role="alert">只能管理自己的直属下属渠道。</p> : null}
    {allowed ? <><ChannelOnboardingPanel channelID={id}/><ChannelModelsPanel channelID={id} delegated />{viewer.roles.includes("channel_admin") ? <ChannelAdminsPanel channelID={id} code={item.code} /> : null}{viewer.roles.some((role) => ["channel_admin", "oem_ops", "oem_audit"].includes(role)) ? <><ChannelUsers channelID={id} />{viewer.roles.includes("channel_admin") ? <ProfessionalCustomerList channelID={id} surface="channel"/>:null}</> : null}</> : null}
  </div>;
}
