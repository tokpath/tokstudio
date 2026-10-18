"use client";
import { useI18n } from "@/components/i18n";
import { useSearchParams } from "next/navigation";
import { useState } from "react";
import Link from "next/link";
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
 return <main className="mx-auto max-w-lg px-5 py-14"><Heading title={t("接受经营人员邀请")} description={t("邀请邮箱与经营范围由邮件中的验证凭据确定。经营人员使用独立于客户账号的密码。")}/><Card><form className="space-y-5" onSubmit={async e=>{e.preventDefault();await command.run({token,password,display_name:name});}}><Notice>{t("已有经营人员账号请使用原密码；首次加入请设置至少 12 位密码。接受后通过经营人员登录进入已授权范围。")}</Notice><fieldset disabled={command.busy||command.unresolved} className="space-y-5"><label className="block text-sm">{t("显示名称")}<Input className="mt-2" value={name} onChange={e=>setName(e.target.value)} required/></label><label className="block text-sm">{t("经营人员密码")}<Input className="mt-2" type="password" autoComplete="new-password" minLength={12} value={password} onChange={e=>setPassword(e.target.value)} required/></label></fieldset>{!!command.error&&<ErrorPanel error={command.error}/>}<Button type="submit" disabled={!token||command.busy}>{command.unresolved?t("重试原邀请确认"):t("验证邮箱并接受邀请")}</Button>{command.result&&<Notice>{t("邀请已接受。")}<Link className="ml-2 underline" href="/login">{t("使用经营人员身份登录")}</Link></Notice>}</form></Card></main>
}
