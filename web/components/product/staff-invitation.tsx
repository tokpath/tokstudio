"use client";
import { useI18n } from "@/components/i18n";
import { useSearchParams } from "next/navigation";
import { useState } from "react";
import Link from "next/link";
import { passwordPolicy } from "@/lib/password";
import { CommandResultSchema } from "@/lib/contracts";
import { useCommand, Heading, Notice, ErrorPanel } from "./shared";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
export function StaffInvitation() {
  const { t } = useI18n();
 const token = useSearchParams().get("token") || "";
 const [name,setName] = useState(""), [password,setPassword] = useState("");
 const command = useCommand("/api/v1/auth/staff-invitations/accept",CommandResultSchema);
 return <main className="mx-auto max-w-lg px-5 py-14"><Heading title={t("接受经营人员邀请")} description={t("邀请链接已确定邮箱和经营范围。请核对邀请来源后接受。")}/><Card><form className="space-y-5" onSubmit={async e=>{e.preventDefault();await command.run({token,password,display_name:name});}}><Notice>{t("已有经营人员账号请使用原密码。新账号请设置符合以下规则的密码。")} {t(passwordPolicy)}</Notice><fieldset disabled={command.busy||command.unresolved} className="space-y-5"><label className="block text-sm">{t("显示名称")}<Input className="mt-2" value={name} onChange={e=>setName(e.target.value)}/></label><label className="block text-sm">{t("经营人员密码")}<Input className="mt-2" type="password" autoComplete="new-password" value={password} onChange={e=>setPassword(e.target.value)} required/></label></fieldset>{!!command.error&&<ErrorPanel error={command.error}/>}<Button type="submit" disabled={!token||command.busy}>{command.unresolved?t("重试原邀请确认"):t("接受邀请")}</Button>{command.result&&<Notice>{t("邀请已接受。")}<Link className="ml-2 underline" href="/login">{t("前往登录")}</Link></Notice>}</form></Card></main>
}
