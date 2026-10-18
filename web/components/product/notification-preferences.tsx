"use client";
import { useI18n } from "@/components/i18n";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { request } from "@/lib/api-client";
import { NotificationPreferencesSchema, CommandResultSchema } from "@/lib/contracts";
import { ErrorPanel, Loading, useCommand } from "./shared";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
export function NotificationPreferences(){const query=useQuery({queryKey:["notification-preferences"],queryFn:()=>request("/api/v1/me/notifications/preferences",NotificationPreferencesSchema)});return query.isPending?<Loading/>:query.isError?<ErrorPanel error={query.error} retry={()=>void query.refetch()}/>:<PreferencesForm key={JSON.stringify(query.data)} initial={query.data}/>;}
function PreferencesForm({initial}:{initial:ReturnType<typeof NotificationPreferencesSchema.parse>}){
  const { t } = useI18n();const [marketing,setMarketing]=useState(initial.marketing),[usage,setUsage]=useState(initial.usage_email);const command=useCommand("/api/v1/me/notifications/preferences",CommandResultSchema);return <Card className="mb-5"><form className="space-y-4" onSubmit={async e=>{e.preventDefault();await command.run({marketing,usage_email:usage});}}><h2 className="font-semibold">{t("邮件授权")}</h2><fieldset disabled={command.busy||command.unresolved} className="flex flex-wrap gap-5 text-sm"><label><input type="checkbox" checked={marketing} onChange={e=>setMarketing(e.target.checked)} className="mr-2"/>{t("本人授权营销邮件（可随时撤回）")}</label><label><input type="checkbox" checked={usage} onChange={e=>setUsage(e.target.checked)} className="mr-2"/>{t("用量邮件提醒")}</label></fieldset><p className="text-xs text-ink-mute">{t("必要的资金、资产、安全与履约通知继续发送。修改会立即作用于尚未发送的邮件。")}</p>{!!command.error&&<ErrorPanel error={command.error}/>}<Button size="sm" type="submit" disabled={command.busy}>{command.unresolved?t("重试原授权修改"):t("保存邮件授权")}</Button></form></Card>}
