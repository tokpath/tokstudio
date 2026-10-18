"use client";
import Link from "next/link";
import { useState } from "react";
import { useRouter } from "next/navigation";
import { useQueryClient } from "@tanstack/react-query";
import { useI18n } from "@/components/i18n";
import { CommandResultSchema, SessionSchema } from "@/lib/contracts";
import { request, setCSRFToken } from "@/lib/api-client";
import { operatorLanding } from "@/lib/navigation";
import { useProduct } from "./context";
import { Heading, Notice, ErrorPanel, Loading, useCommand } from "./shared";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
export function StaffPassword() {
 const { t } = useI18n(), { session, sessionError, reload } = useProduct();
 const router = useRouter(), client = useQueryClient();
 const [current, setCurrent] = useState(""), [password,setPassword] = useState(""), [confirm,setConfirm] = useState("");
 const [refreshError,setRefreshError] = useState<unknown>();
 const command = useCommand("/api/v1/auth/staff-password",CommandResultSchema);
 async function enter() {
  try {
   const next = await request("/api/v1/auth/me",SessionSchema);
   client.setQueryData(["session"],next);
   if (next.kind === "staff") { setCSRFToken(next.csrf_token); router.replace(next.workspaces.length ? operatorLanding(next.workspaces[0]) : "/"); }
  } catch (error) { setRefreshError(error); }
 }
 if (sessionError) return <ErrorPanel error={sessionError} retry={reload}/>;
 if (!session) return <Loading/>;
 if (session.kind !== "staff") return <main className="mx-auto max-w-lg px-5 py-14"><Notice>{t("请先登录经营人员账号。")}<Link href="/login" className="ml-2 underline">{t("前往登录")}</Link></Notice></main>;
 return <main className="mx-auto max-w-lg px-5 py-14"><Heading title={t(session.requires_password_change ? "修改初始密码" : "修改密码")} description={t("请设置只有你自己知道的密码。修改后，其他已登录设备需要重新登录。")}/><Card><form className="space-y-5" onSubmit={async e=>{e.preventDefault(); if (password!==confirm || password===current) return; const result=await command.run({current_password:current,new_password:password}); if(result){setCurrent("");setPassword("");setConfirm("");await enter();}}}>
 <fieldset className="space-y-5" disabled={command.busy||command.unresolved||!!command.result}>
 <label className="block text-sm">{t("当前密码")}<Input className="mt-2" type="password" autoComplete="current-password" required value={current} onChange={e=>setCurrent(e.target.value)}/></label>
 <label className="block text-sm">{t("新密码")}<Input className="mt-2" type="password" autoComplete="new-password" minLength={10} required value={password} onChange={e=>setPassword(e.target.value)}/><span className="mt-2 block text-xs text-ink-mute">{t("至少 10 位，且不能与当前密码相同。")}</span></label>
 <label className="block text-sm">{t("确认新密码")}<Input className="mt-2" type="password" autoComplete="new-password" minLength={10} required value={confirm} onChange={e=>setConfirm(e.target.value)}/></label>
 {confirm&&password!==confirm&&<p role="alert" className="text-sm text-danger">{t("两次输入的密码不一致。")}</p>}
 </fieldset>{!!command.error&&<ErrorPanel error={command.error}/>} {!!refreshError&&<ErrorPanel error={refreshError} retry={()=>void enter()}/>}
 {command.result?<Notice>{t("密码已更新。")}</Notice>:<Button type="submit" disabled={command.busy||!current||password.length<10||password!==confirm||password===current}>{t(command.unresolved?"重试本次操作":"保存并进入经营空间")}</Button>}
 </form></Card></main>;
}
