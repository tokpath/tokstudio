"use client";
import { useI18n } from "@/components/i18n";
import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { z } from "zod";
import { PublicContextSchema, RegistrationResultSchema, LoginResultSchema, CommandResultSchema } from "@/lib/contracts";
import { authContinuation, safeCustomerNext } from "@/lib/customer-next";
import { operatorLanding } from "@/lib/navigation";
import { request, setCSRFToken } from "@/lib/api-client";
import { useProduct } from "./context";
import { ErrorPanel, Heading, Notice, useCommand } from "./shared";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ArrowUpRight, KeyRound, Layers3, ReceiptText } from "lucide-react";

export function AuthPage({ mode }: { mode: "login" | "register" | "verify" | "forgot" | "reset" }) {
  const { t } = useI18n();
  const router = useRouter(), search = useSearchParams(), queryClient = useQueryClient();
  const { context } = useProduct();
  const next = safeCustomerNext(search.get("next"));
  const invite = search.get("invite_code") || search.get("invite") || undefined;
  const [kind, setKind] = useState<"customer" | "staff" | undefined>(undefined), [code, setCode] = useState((search.get("invite_code") || search.get("invite") || "").trim().toUpperCase()), [email, setEmail] = useState(""), [password, setPassword] = useState(""), [name, setName] = useState("");
  const login = useCommand("/api/v1/auth/login", LoginResultSchema);
  const [choices, setChoices] = useState<string[]>([]);
  const register = useCommand("/api/v1/auth/registrations", RegistrationResultSchema);
  const verify = useCommand("/api/v1/auth/verify", CommandResultSchema);
  const forgot = useCommand("/api/v1/auth/password-resets", CommandResultSchema);
  const reset = useCommand("/api/v1/auth/password-resets/complete", CommandResultSchema);
  const google = useCommand("/api/v1/auth/google/start", z.object({ url: z.string() }));
  const attribution = useQuery({ queryKey: ["registration-context", code], queryFn: () => request(`/api/v1/public/context?invite_code=${encodeURIComponent(code)}`, PublicContextSchema), enabled: mode === "register" && !!code });
  const title = { login: t("登录你的空间"), register: t("创建客户账号"), verify: t("验证邮箱"), forgot: t("找回密码"), reset: t("设置新密码") }[mode];
  const action = mode === "login" ? login : mode === "register" ? register : mode === "verify" ? verify : mode === "forgot" ? forgot : reset;
  const invalidCode = !!code && (attribution.isPending || attribution.isError || attribution.data?.invitation?.status !== "valid");
  const targetOperator = code ? attribution.data?.invitation?.operator || attribution.data?.operator : context?.operator;
  const token = search.get("token") || "";
  return <main className="th-auth-layout"><aside className="th-auth-intro"><span aria-hidden="true" className="th-auth-mark">{(context?.brand.name || "TokenHub").slice(0, 1)}</span><p className="th-eyebrow mb-5 text-brand-emphasis">{context?.brand.name || "TokenHub"}</p><h2 className="text-[38px] font-semibold leading-[1.2] tracking-tight">{t("一个品牌入口，")}<br />{t("连接你的模型与应用。")}</h2><div className="mt-8 space-y-5">{[{ icon: Layers3, title: "一个额度池，多种模型" }, { icon: KeyRound, title: "为每个应用单独建 Key" }, { icon: ReceiptText, title: "从请求追溯每笔费用" }].map(item => <p key={item.title} className="flex items-center gap-3 text-sm text-ink-secondary"><item.icon className="size-4 text-brand-emphasis" />{t(item.title)}</p>)}</div><Link href="/models" className="mt-10 inline-flex w-fit items-center gap-2 text-sm font-medium text-brand-emphasis">{t("探索模型与价格")}<ArrowUpRight className="size-4" /></Link></aside><div className="th-auth-card"><Card><Heading eyebrow={context?.brand.name} title={t(title)} description={mode === "register" ? t("正式注册完成后确定经营归属和邀请关系，请核对后再提交。") : t("输入邮箱和密码，系统将自动进入你可使用的空间。")} />
    {context?.google_sign_in && (mode === "login" || mode === "register") && <div className="mb-6 space-y-3"><Button type="button" variant="outline" className="w-full" disabled={google.busy || (mode === "register" && invalidCode)} onClick={async () => { const result = await google.run({ ...(code ? { invite_code: code } : {}), ...(next ? { next } : {}) }); if (result?.url) window.location.assign(result.url); }}>{google.busy ? t("正在前往 Google…") : t("使用 Google 账号继续")}</Button>{!!google.error && <ErrorPanel error={google.error} />}<p className="text-xs leading-6 text-ink-mute">{t("Google 登录只进入当前品牌。新账号按本页归属注册；已经用密码注册的邮箱仍使用密码登录。")}</p><div className="flex items-center gap-3 pt-2 text-[11px] text-ink-mute"><span className="h-px flex-1 bg-hairline" /><span>{t("使用邮箱继续")}</span><span className="h-px flex-1 bg-hairline" /></div></div>}
    <form className="space-y-5" onSubmit={async event => {
      event.preventDefault();
      if (mode === "login") { const result = await login.run({ kind, email, password }); if (result?.kind === "workspace_choice") { setChoices(result.choices); setKind(undefined); } if (result && result.kind !== "anonymous" && result.kind !== "workspace_choice") { queryClient.removeQueries({ predicate: query => !["public-context", "session"].includes(String(query.queryKey[0])) }); queryClient.setQueryData(["session"], result); setCSRFToken(result.csrf_token); } if (result?.kind === "customer") { router.push(next || "/workspace"); } else if (result?.kind === "staff") router.push(result.requires_password_change ? "/change-password" : result.workspaces.length ? operatorLanding(result.workspaces.find(item => item.operator.id === result.active_operator_id) || result.workspaces[0]) : "/login"); }
      if (mode === "register" && !invalidCode) await register.run({ email, password, display_name: name, ...(code ? { invite_code: code } : {}), ...(next ? { next } : {}) });
      if (mode === "verify") await verify.run({ token });
      if (mode === "forgot") await forgot.run({ email });
      if (mode === "reset") await reset.run({ token, password });
    }}>
      {mode === "login" && choices.length > 0 && <fieldset className="flex gap-4 rounded-control border border-hairline p-3 text-sm"><legend className="px-1 text-ink-mute">{t("此账号可进入两个空间，请选择本次要办理的业务")}</legend>{[["customer", t("客户服务")], ["staff", t("经营管理")]].filter(([value]) => choices.includes(value)).map(([value, label]) => <label key={value} className="flex items-center gap-2"><input type="radio" checked={kind === value} onChange={() => setKind(value as "customer" | "staff")} disabled={action.busy || action.unresolved} />{t(label)}</label>)}</fieldset>}
      {mode === "register" && <label className="block text-sm">{t("显示名称")}<Input value={name} onChange={e => setName(e.target.value)} autoComplete="nickname" required maxLength={80} disabled={action.busy || action.unresolved} className="mt-2" /></label>}
      {["login", "register", "forgot"].includes(mode) && <label className="block text-sm">{t("邮箱")}<Input type="email" value={email} onChange={e => { setEmail(e.target.value); setChoices([]); setKind(undefined); }} autoComplete="email" required disabled={action.busy || action.unresolved} className="mt-2" /></label>}
      {["login", "register", "reset"].includes(mode) && <label className="block text-sm">{t("密码")}<Input type="password" value={password} onChange={e => { setPassword(e.target.value); setChoices([]); setKind(undefined); }} autoComplete={mode === "login" ? "current-password" : "new-password"} minLength={mode === "login" ? undefined : 12} required disabled={action.busy || action.unresolved} className="mt-2" /></label>}
      {mode === "register" && <div><label className="block text-sm">{t("邀请码（可选）")}<Input autoCapitalize="characters" spellCheck={false} placeholder={t("例如 A1B2C3")} value={code} onChange={e => setCode(e.target.value.trim().toUpperCase())} autoComplete="off" disabled={action.busy || action.unresolved} className="mt-2" /></label><p className="mt-2 text-xs text-ink-mute">{t("6位数字或大写字母，小写输入会自动转为大写。")}</p>{code && <button type="button" onClick={() => setCode("")} disabled={action.busy || action.unresolved} className="mt-2 text-xs text-brand-emphasis underline">{t("显式移除邀请码，改为品牌直属注册")}</button>}{attribution.isError && <div className="mt-3"><ErrorPanel error={attribution.error} retry={() => void attribution.refetch()} /></div>}{code && attribution.data?.invitation?.status !== "valid" && !attribution.isPending && <p role="alert" className="mt-3 text-sm text-danger">{t(attribution.data?.invitation?.message || "邀请码无效，请更正或显式移除。")}</p>}<div className="mt-4"><Notice>{t("品牌：")}{context?.brand.name || t("正在核实")}<br />{t("经营方：")}{targetOperator?.name || t("正在核实")}<br />{t("邀请人：")}{code && attribution.data?.invitation?.type === "user" ? attribution.data.invitation.inviter_display_name || t("已核实的邀请人") : t("无用户邀请人")}</Notice></div></div>}
      {mode === "verify" && <Notice>{token ? t("点击下方按钮验证本次邮箱注册。重复验证不会重复建立账号或发放奖励。") : t("链接缺少验证凭据，请从邮箱中的完整链接进入。")}</Notice>}
      {!!action.error && <ErrorPanel error={action.error} />}
      {action.unresolved && <Notice>{t("上次操作结果尚未核实；输入已锁定，再次提交会沿用原操作编号。")}{action.actionKey && <span className="block break-all font-mono text-xs">{action.actionKey}</span>}</Notice>}
      <Button type="submit" className="w-full" disabled={action.busy || (mode === "login" && choices.length > 0 && !kind) || (mode === "register" && invalidCode) || (["verify", "reset"].includes(mode) && !token)}>{action.busy ? t("正在处理…") : action.unresolved ? t("重试本次操作") : { login: t("登录"), register: t("确认归属并注册"), verify: t("验证邮箱"), forgot: t("发送找回邮件"), reset: t("更新密码") }[mode]}</Button>
      {register.result && <Notice>{t(register.result.message || "")}</Notice>}{verify.result && <Notice>{t(verify.result.message || "邮箱验证已完成。")}<Link href={authContinuation("/login", next)} className="ml-2 underline">{t("前往登录")}</Link></Notice>}{forgot.result && <Notice>{t(forgot.result.message || "如该账号可找回，验证邮件将发送到对应邮箱。")}</Notice>}{reset.result && <Notice>{t(reset.result.message || "密码已更新。")}<Link href={authContinuation("/login", next)} className="ml-2 underline">{t("重新登录")}</Link></Notice>}
    </form>
    {mode === "login" && <div className="mt-6 flex justify-between text-sm text-brand-emphasis"><Link href={authContinuation("/forgot-password", next)}>{t("忘记密码")}</Link><Link href={authContinuation("/register", next, invite)}>{t("创建客户账号")}</Link></div>}
  </Card></div></main>;
}
