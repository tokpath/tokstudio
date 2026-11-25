"use client";

import { useState, type FormEvent } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import type { z } from "zod";
import { useI18n } from "@/components/i18n";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { request } from "@/lib/api-client";
import { CommandResultSchema, SubscriptionAccountSchema, SubscriptionOAuthStartSchema, SubscriptionPoolSchema, SubscriptionPoolsSchema, SubscriptionVendorSchema, type SubscriptionVendor } from "@/lib/contracts";
import { canReadResource, operatorResourcePath } from "@/lib/navigation";
import { subscriptionVendors } from "@/lib/subscription-vendors";
import { useProduct } from "./context";
import { OperatorPanel } from "./operator-resources";
import { Empty, ErrorPanel, Heading, Loading, Money, Status, Table, When, useCommand } from "./shared";

type Pool = z.infer<typeof SubscriptionPoolSchema>;
type Account = z.infer<typeof SubscriptionAccountSchema>;

type Selection =
  | { mode: "pool"; pool?: Pool }
  | { mode: "account"; pool: Pool; account?: Account }
  | { mode: "remove"; pool: Pool; account: Account };

const accountStates = { paused: "已停用", cooldown: "冷却中", reauth_required: "需重新授权", subscription_expired: "订阅已到期", busy: "调用中" };
const oauthMessages: Record<string, string> = {
  saved: "账号已授权",
  cancelled: "已取消授权",
  duplicate: "此账号已在当前账号池中。",
  conflict: "账号已被修改，请刷新后重新授权。",
  failed: "授权未完成，请在账号池中重试。",
};
const selectClass = "mt-2 min-h-11 w-full rounded-control border border-hairline bg-canvas-raised px-3";

export function SubscriptionPoolsScreen({ operatorID }: { operatorID: string }) {
  const { t } = useI18n();
  const search = useSearchParams();
  const { session } = useProduct();
  const workspace = session?.kind === "staff" ? session.workspaces.find(item => item.operator.id === operatorID) : undefined;
  const canRead = canReadResource(workspace, "subscription-pools");
  const path = `/api/v1/operators/${encodeURIComponent(operatorID)}/subscription-pools`;
  const command = useCommand(path, CommandResultSchema);
  const oauth = useCommand(path + "/oauth/start", SubscriptionOAuthStartSchema);
  const data = useQuery({ queryKey: ["subscription-pools", operatorID], queryFn: ({ signal }) => request(path, SubscriptionPoolsSchema, { signal }), enabled: canRead, refetchInterval: 30_000 });
  const [selection, setSelection] = useState<Selection>();
  const [saved, setSaved] = useState(false);
  const blocked = command.busy || command.unresolved || oauth.busy || oauth.unresolved;
  const open = (next: Selection) => {
    command.reset();
    oauth.reset();
    setSaved(false);
    setSelection(next);
    if (search.has("oauth")) {
      const url = new URL(window.location.href);
      url.searchParams.delete("oauth");
      window.history.replaceState(null, "", url);
    }
  };
  const close = () => { if (!blocked) { command.reset(); oauth.reset(); setSelection(undefined); } };
  const save = async (payload: object) => {
    if (await command.run(payload)) { setSelection(undefined); setSaved(true); }
  };
  const authorize = async (payload: object) => {
    const authorization = await oauth.run(payload);
    if (authorization) window.location.assign(authorization.url);
  };
  if (!canRead) return <Empty title={t("没有查看权限")}>{t("此页面需要当前经营方明确授予相应权限。")}</Empty>;
  const pools = data.data?.items ?? [];
  const oauthStatus = search.get("oauth") || "";
  const oauthMessage = Object.hasOwn(oauthMessages, oauthStatus) ? oauthMessages[oauthStatus] : undefined;
  return <>
    <Heading title={t("订阅账号池")} description={t("管理上游订阅账号和可用状态。账号池可以用作供应商模型的连接。")} actions={
      <Button type="button" onClick={() => open({ mode: "pool" })}><Plus aria-hidden />{t("新建账号池")}</Button>
    } />
    {saved && <p role="status" className="mb-4 text-sm text-success">{t("已保存")}</p>}
    {oauthMessage && <p role={oauthStatus === "saved" || oauthStatus === "cancelled" ? "status" : "alert"} className={`mb-4 text-sm ${oauthStatus === "saved" ? "text-success" : "text-ink-secondary"}`}>{t(oauthMessage)}</p>}
    {data.isPending ? <Loading /> : data.isError ? <ErrorPanel error={data.error} retry={() => void data.refetch()} /> : !pools.length ?
      <Empty title={t("暂无订阅账号池")}>{t("新建账号池并加入一个已授权的订阅账号即可开始。")}</Empty> :
      <div className="space-y-4">{pools.map(pool => <Card key={pool.id}>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-base font-semibold [overflow-wrap:anywhere]">{pool.name}</h2>
            <p className="mt-1 text-sm text-ink-mute">{subscriptionVendors[pool.subscription_kind].label} · {t("{available} / {total} 个账号可用", {
              available: pool.enabled ? pool.accounts.filter(account => account.status === "available").length : 0, total: pool.accounts.length,
            })}</p>
          </div>
          <Status value={pool.enabled ? "enabled" : "disabled"} />
        </div>
        <details open={pools.length === 1 || search.get("pool") === pool.id} className="mt-4 border-t border-hairline pt-3">
          <summary className="w-fit cursor-pointer py-2 text-sm font-medium">{t("账号管理")}</summary>
          <div className="my-3 flex flex-wrap gap-2">
            <Button type="button" size="sm" onClick={() => open({ mode: "account", pool })}>{t("添加账号")}</Button>
            <Button type="button" size="sm" variant="outline" onClick={() => open({ mode: "pool", pool })}>{t("编辑账号池")}</Button>
            <Button asChild size="sm" variant="outline"><Link href={`${operatorResourcePath(operatorID, "providers")}?pool=${encodeURIComponent(pool.id)}&pool_kind=${pool.subscription_kind}`}>{t("添加供应商模型")}</Link></Button>
            {canReadResource(workspace, "supplier-costs") && <Button asChild size="sm" variant="ghost"><Link href={operatorResourcePath(operatorID, "supplier-costs")}>{t("真实上游成本")}</Link></Button>}
          </div>
          {pool.accounts.length ? <Table rows={pool.accounts} getID={account => account.id} columns={[
            { label: t("账号名称"), cell: account => <span className="font-medium [overflow-wrap:anywhere]">{account.name}</span> },
            { label: t("状态"), cell: account => <><Status value={account.status} labels={accountStates} />{account.status === "cooldown" && <p className="mt-1 text-xs text-ink-mute"><When value={account.cooldown_until} /></p>}</> },
            { label: t("订阅到期"), cell: account => <When value={account.subscription_ends_at} /> },
            { label: t("操作"), cell: account => <div className="flex flex-wrap gap-1">
              <Button size="sm" variant="ghost" onClick={() => open({ mode: "account", pool, account })}>{t(account.status === "reauth_required" ? "重新授权" : "编辑")}</Button>
              <Button size="sm" variant="ghost" onClick={() => open({ mode: "remove", pool, account })}>{t("移除")}</Button>
            </div> },
          ]} /> : <p className="py-4 text-sm text-ink-mute">{t("还没有账号。")}</p>}
          {pool.latest_statement && <div className="mt-4 flex flex-wrap items-center justify-between gap-2 border-t border-hairline pt-3 text-sm">
            <div className="flex min-w-0 flex-wrap gap-x-2">
              <span className="text-ink-mute">{t("最近登记的订阅账单")}</span>
              <Link className="text-brand-emphasis underline [overflow-wrap:anywhere]" href={`${operatorResourcePath(operatorID, "supplier-costs")}?record=${encodeURIComponent(pool.latest_statement.id)}`}>{pool.latest_statement.statement_reference}</Link>
            </div>
            <Money value={pool.latest_statement.amount} unit={pool.latest_statement.currency} />
          </div>}
        </details>
      </Card>)}</div>}
    {selection && <OperatorPanel open blocked={blocked} close={close} description="" title={t(selection.mode === "pool" ? selection.pool ? "编辑账号池" : "新建账号池" : selection.mode === "remove" ? "移除账号" : selection.account ? "编辑账号" : "添加账号")}>
      {selection.mode === "pool" ? <PoolForm pool={selection.pool} blocked={blocked} save={save} /> :
        selection.mode === "account" ? <AccountForm pool={selection.pool} account={selection.account} blocked={blocked} save={save} authorize={authorize} /> :
          <div className="space-y-4">
            <p className="text-sm text-ink-secondary">{t("移除「{name}」后，账号将退出调度，已保存的授权凭据同时清除。", { name: selection.account.name })}</p>
            <Button type="button" variant="destructive" disabled={blocked} onClick={() => void save({
              action: "remove_account",
              account: { id: selection.account.id, pool_id: selection.pool.id, expected_version: selection.account.version },
            })}>{t("确认移除")}</Button>
          </div>}
      {!!command.error && <div className="mt-4"><ErrorPanel error={command.error} retry={command.unresolved ? () => void save({}) : undefined} /></div>}
      {!!oauth.error && <div className="mt-4"><ErrorPanel error={oauth.error} retry={oauth.unresolved ? () => void authorize({}) : undefined} /></div>}
    </OperatorPanel>}
  </>;
}

function PoolForm({ pool, blocked, save }: { pool?: Pool; blocked: boolean; save: (body: object) => Promise<void> }) {
  const { t } = useI18n();
  const [vendor, setVendor] = useState<SubscriptionVendor>(pool?.subscription_kind || "codex");
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const values = new FormData(event.currentTarget);
    const credential = String(values.get("credential") || "").trim();
    void save({ action: "save_pool", pool: {
      id: pool?.id, name: values.get("name"), subscription_kind: vendor,
      enabled: !pool || values.get("enabled") === "on", expected_version: pool?.version ?? 0,
      first_account: credential ? { name: t("首个账号"), credential } : undefined,
    } });
  };
  return <form className="space-y-4" onSubmit={submit}>
    <label className="block text-sm">{t("账号池名称")}<Input name="name" defaultValue={pool?.name} className="mt-2" required maxLength={80} disabled={blocked} /></label>
    {pool ? <p className="text-sm text-ink-secondary">{subscriptionVendors[vendor].label}</p> :
      <label className="block text-sm">{t("供应商")}<select name="vendor" className={selectClass} value={vendor} disabled={blocked} onChange={event => setVendor(SubscriptionVendorSchema.parse(event.target.value))}>
        {Object.entries(subscriptionVendors).map(([value, supplier]) => <option value={value} key={value}>{supplier.label}</option>)}
      </select></label>}
    {pool ? <label className="flex items-center gap-2 text-sm"><input name="enabled" type="checkbox" defaultChecked={pool.enabled} disabled={blocked} />{t("启用")}</label> :
      <CredentialField vendor={vendor} blocked={blocked} optional />}
    <Button type="submit" disabled={blocked}>{t("保存")}</Button>
  </form>;
}

function AccountForm({ pool, account, blocked, save, authorize }: { pool: Pool; account?: Account; blocked: boolean; save: (body: object) => Promise<void>; authorize: (body: object) => Promise<void> }) {
  const { t } = useI18n();
  const [mode, setMode] = useState<"settings" | "browser" | "import">(account && account.status !== "reauth_required" ? "settings" : "browser");
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const values = new FormData(event.currentTarget);
    const endsAt = String(values.get("ends_at") || "");
    const draft = {
      id: account?.id, pool_id: pool.id, name: values.get("name"),
      subscription_ends_at: endsAt ? new Date(endsAt).toISOString() : null,
      enabled: values.get("enabled") === "on", expected_version: account?.version ?? 0,
    };
    if (mode === "browser") {
      if (pool.browser_oauth) void authorize(draft);
    } else {
      void save({ action: "save_account", account: { ...draft, credential: String(values.get("credential") || "").trim() } });
    }
  };
  const end = account?.subscription_ends_at ? new Date(account.subscription_ends_at) : undefined;
  const localEnd = end ? new Date(end.getTime() - end.getTimezoneOffset() * 60_000).toISOString().slice(0, 16) : "";
  return <form className="space-y-4" onSubmit={submit}>
    <label className="block text-sm">{t("账号名称")}<Input name="name" className="mt-2" defaultValue={account?.name} disabled={blocked} required maxLength={80} /></label>
    {mode === "import" && <CredentialField vendor={pool.subscription_kind} blocked={blocked} />}
    <label className="block text-sm">{t("订阅到期（可选）")}<Input name="ends_at" type="datetime-local" className="mt-2" defaultValue={localEnd} disabled={blocked} /></label>
    <label className="flex items-center gap-2 text-sm"><input name="enabled" type="checkbox" defaultChecked={account?.enabled ?? true} disabled={blocked} />{t("启用")}</label>
    {mode === "browser" && !pool.browser_oauth && <p className="text-sm text-ink-mute">{t("此供应商的浏览器授权尚未配置。")}</p>}
    <div className="flex flex-wrap items-center gap-2">
      <Button type="submit" disabled={blocked || mode === "browser" && !pool.browser_oauth}>{t(mode === "browser" ? "在浏览器中授权" : "保存")}</Button>
      {mode !== "browser" && pool.browser_oauth && <Button type="button" variant="outline" disabled={blocked} onClick={() => setMode("browser")}>{t("在浏览器中授权")}</Button>}
      {mode !== "import" && <Button type="button" variant="ghost" disabled={blocked} onClick={() => setMode("import")}>{t("导入凭据")}</Button>}
      {mode === "import" && account && account.status !== "reauth_required" && <Button type="button" variant="ghost" disabled={blocked} onClick={() => setMode("settings")}>{t("取消")}</Button>}
    </div>
  </form>;
}

function CredentialField({ vendor, blocked, optional }: { vendor: SubscriptionVendor; blocked: boolean; optional?: boolean }) {
  const { t } = useI18n();
  const hint = vendor === "codex" ? "粘贴已授权的 Codex auth.json 内容。" :
    vendor === "claude" ? "粘贴已授权的 Claude Code Setup Token 或授权 JSON。" : "粘贴供应商授权的凭据 JSON。";
  return <label className="block text-sm">
    {t(optional ? "订阅授权（可选）" : "订阅授权")}
    <Input name="credential" type="password" autoComplete="off" spellCheck={false} className="mt-2" maxLength={65536} disabled={blocked} required={!optional} />
    <span className="mt-2 block text-xs leading-5 text-ink-mute">{t(hint)}</span>
  </label>;
}
