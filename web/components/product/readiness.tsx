"use client";

import Link from "next/link";
import { z } from "zod";
import { useI18n } from "@/components/i18n";
import { canReadResource } from "@/lib/navigation";

export const ReadinessRequirementSchema = z.object({ code: z.string(), description: z.string(), ready: z.boolean(), resource: z.string().optional(), action_hint: z.string().optional() });
export const ReadinessSchema = z.array(z.object({ feature: z.string(), requirements: z.array(ReadinessRequirementSchema) }));

export function ReadinessAction({ requirement, workspace, operatorID, label = "去配置" }: {
  requirement: z.infer<typeof ReadinessRequirementSchema>;
  workspace: Parameters<typeof canReadResource>[0];
  operatorID: string;
  label?: string;
}) {
  const { t } = useI18n();
  const resource = requirement.resource;
  if (resource && canReadResource(workspace, resource)) return <Link className="shrink-0 text-sm text-brand-emphasis underline" href={`/manage/${encodeURIComponent(operatorID)}/${resource}`}>{t(label)}</Link>;
  let hint = requirement.action_hint;
  if (!hint && requirement.code === "monetary_foundation") hint = workspace?.operator.kind === "platform" ? "请由平台负责人完成基础设置并等待生效。" : "等待平台负责人完成清算币种和 ATG 基础比例设置并生效。";
  if (!hint && requirement.code === "verification_mail" && workspace?.operator.kind === "channel") hint = "等待所属品牌方配置可用的邮件服务。";
  if (!hint && resource && workspace && !canReadResource({ ...workspace, permissions: ["*"] }, resource)) hint = "等待平台完成配置。";
  return <span className="text-xs leading-5 text-ink-mute">{t(hint || "请联系本经营方负责人授权或办理。")}</span>;
}
