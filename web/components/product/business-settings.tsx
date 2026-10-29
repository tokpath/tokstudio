"use client";

import { useState } from "react";
import Link from "next/link";
import { z } from "zod";
import { useI18n } from "@/components/i18n";
import { canReadResource } from "@/lib/navigation";
import { CommandResultSchema, type Entity } from "@/lib/contracts";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { useProduct } from "./context";
import { Empty, ErrorPanel, Notice, useCommand } from "./shared";
import { MonetarySettings } from "./monetary-settings";

const checksSchema = z.array(z.object({ feature: z.string(), requirements: z.array(z.object({ code: z.string(), description: z.string(), ready: z.boolean(), resource: z.string().optional() })) }));
const features = [
  ["registration", "新客户注册", "允许新客户注册和建立邀请关系。"],
  ["new_purchase", "套餐新购", "允许客户购买新套餐，暂停不会取消已经购买的权益。"],
  ["renewal", "套餐续费", "允许执行客户已授权的续费，客户仍可自行取消。"],
  ["model_usage", "模型调用", "允许使用 API 和网页体验，已受理请求继续按原规则处理。"],
  ["activity", "营销活动", "允许客户参加活动，已有订单保留成交优惠。"],
  ["settlement", "资金结算", "办理存量退款与佣金等资金责任，和新增销售分别控制。"],
] as const;

export function BusinessSettings({ row, operatorID }: { row?: Entity; operatorID: string }) {
  const { t } = useI18n();
  const { session } = useProduct();
  const workspace = session?.kind === "staff" ? session.workspaces.find(item => item.operator.id === operatorID) : undefined;
  const command = useCommand(`/api/v1/operators/${encodeURIComponent(operatorID)}/business-status`, CommandResultSchema);
  const [pending, setPending] = useState<string>();
  const checks = checksSchema.safeParse(row?.values.readiness);
  if (!row) return <Empty>{t("营业状态尚未读取，请刷新后重试。")}</Empty>;
  async function change(feature: string, enabled: boolean) {
    if (!row || command.busy || command.unresolved) return;
    command.reset(); setPending(feature);
    const result = await command.run({ feature, enabled, expected_version: row.version });
    if (result) setPending(undefined);
  }
  return <div className="space-y-5">
    <MonetarySettings canManage={workspace?.can_manage_monetary === true} />
    {!!command.error && <ErrorPanel error={command.error} />}
    {command.unresolved && <Notice>{t("正在核实营业设置，请重试原操作或刷新查看最新状态。")}<Button className="ml-3" variant="outline" onClick={() => void command.run(undefined).then(result => { if (result) setPending(undefined); })} disabled={command.busy}>{t("重试原操作")}</Button></Notice>}
    <div className="grid gap-4 lg:grid-cols-2">{features.map(([feature, label, help]) => {
      const enabled = row.values[feature] === true;
      const requirements = checks.success ? checks.data.find(item => item.feature === feature)?.requirements : undefined;
      const missing = requirements?.filter(item => !item.ready) || [];
      const unavailable = !enabled && (!requirements || missing.length > 0);
      return <Card key={feature}><div className="flex items-start justify-between gap-4"><div><h2 className="font-semibold">{t(label)}</h2><p className="mt-2 text-sm leading-6 text-ink-secondary">{t(help)}</p></div><span className={`shrink-0 rounded-full px-2 py-1 text-xs ${enabled ? "bg-success/10 text-success" : "bg-canvas text-ink-mute"}`}>{t(enabled ? "已开放" : "未开放")}</span></div>
        {missing.length > 0 && <ul className="mt-4 space-y-2 rounded-control bg-canvas p-3">{missing.map(item => <li key={item.code} className="flex items-center justify-between gap-3 text-sm"><span>{t(item.description)}</span>{item.resource && canReadResource(workspace, item.resource) ? <Link className="shrink-0 text-brand-emphasis underline" href={`/manage/${encodeURIComponent(operatorID)}/${item.resource}`}>{t("去配置")}</Link> : <span className="shrink-0 text-xs text-ink-mute">{t("请联系负责人")}</span>}</li>)}</ul>}
        <div className="mt-5 flex items-center justify-between gap-3 border-t border-hairline pt-4"><span className="text-xs text-ink-mute">{t(unavailable ? "完成所需配置后可开启" : enabled ? "暂停前请留意正在使用此服务的客户。" : "所需配置已就绪")}</span><Button variant={enabled ? "outline" : "default"} disabled={command.busy || command.unresolved || unavailable} aria-label={`${t(enabled ? "暂停" : "开启")} ${t(label)}`} onClick={() => void change(feature, !enabled)}>{command.busy && pending === feature ? t("正在保存…") : t(enabled ? "暂停" : "开启")}</Button></div>
      </Card>;
    })}</div>
    <p className="text-xs leading-6 text-ink-mute">{t("系统自动检查依赖并记录操作者和变更，无需填写检查编号或证明。")}</p>
  </div>;
}
