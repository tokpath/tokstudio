"use client";
import { useQuery } from "@tanstack/react-query";
import { useI18n } from "@/components/i18n";
import { Card } from "@/components/ui/card";
import { request } from "@/lib/api-client";
import { ChannelInvitationSchema } from "@/lib/contracts";
import { canReadResource } from "@/lib/navigation";
import { useProduct } from "./context";
import { CopyButton } from "./customer-ui";
import { ErrorPanel, Loading } from "./shared";

export function ChannelInvitationCard({ operatorID }: { operatorID: string }) {
  const { t } = useI18n();
  const { session } = useProduct();
  const workspace = session?.kind === "staff" ? session.workspaces.find(w => w.operator.id === operatorID) : undefined;
  const allowed = workspace?.operator.kind === "channel" && canReadResource(workspace, "customers");
  const data = useQuery({ queryKey: ["channel-invitation", operatorID], queryFn: ({ signal }) => request(`/api/v1/operators/${encodeURIComponent(operatorID)}/customer-invitation`, ChannelInvitationSchema, { signal }), enabled: allowed });
  if (!allowed) return null;
  return <Card className="mb-6" id="channel-invitation">
    <h2 className="text-lg font-semibold">{t("渠道邀请")}</h2>
    <p className="mt-2 text-sm leading-7 text-ink-secondary">{t("通过此链接或邀请码注册的新用户，将成为{operator}的直属客户，不产生用户邀请上下级关系。", { operator: workspace.operator.name })}</p>
    {data.isPending ? <Loading /> : data.isError ? <div className="mt-4"><ErrorPanel error={data.error} retry={() => void data.refetch()} /></div> : <>
      <div className="mt-4 grid gap-5 lg:grid-cols-[1fr_2fr]">
        <div className="min-w-0"><p className="mb-2 text-xs text-ink-mute">{t("渠道邀请码")}</p><code className="block break-all text-lg font-semibold">{data.data.invite_code}</code>{data.data.enabled && <div className="mt-3"><CopyButton value={data.data.invite_code} label={t("复制渠道邀请码")} /></div>}</div>
        <div className="min-w-0"><p className="mb-2 text-xs text-ink-mute">{t("渠道邀请链接")} · {data.data.brand_name}</p>{data.data.invite_url ? <><p className="break-all text-sm">{data.data.invite_url}</p>{data.data.enabled && <div className="mt-3"><CopyButton value={data.data.invite_url} label={t("复制渠道邀请链接")} /></div>}</> : <p className="text-sm leading-7 text-hold">{t("品牌尚未配置可用域名，请联系品牌方。域名启用后会自动生成邀请链接，不会使用其他品牌的域名。")}</p>}</div>
      </div>
      {!data.data.enabled ? <p className="mt-4 text-sm text-hold">{t("渠道邀请码已停用，请联系品牌方。")}</p> : !data.data.registration_open && <p className="mt-4 text-sm text-hold">{t("当前渠道暂未开放注册。邀请信息可保存，开放注册后再邀请新客户。")}</p>}
    </>}
  </Card>;
}
