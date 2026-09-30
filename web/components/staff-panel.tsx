"use client";

import { useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ConsolePageHeader } from "@/components/console/page-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { useViewer } from "@/components/rbac/viewer-context";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";

type Staff = {
  user_id: string; email: string; display_name: string; roles: string[]; status: "active" | "disabled";
  created_by_email: string; updated_by_email: string; created_at: string; updated_at: string;
};
type StaffList = { items: Staff[]; roles: string[] };
type History = { items: { id: string; action: string; actor_user_id: string; created_at: string; before?: Staff; after?: Staff }[]; actors: Record<string, string> };
type Operation = { mode: "create" } | { mode: "roles" | "disable" | "enable" | "history"; member: Staff };
const roleNames: Record<string, string> = {
  platform_admin: "admin", channel_admin: "admin", ops_admin: "ops", oem_ops: "ops",
  finance_admin: "finance", oem_finance: "finance", tech_admin: "tech", audit_readonly: "audit", oem_audit: "audit",
};
async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${apiBase}${path}`, { credentials: "include", ...init });
  const body = await response.json();
  if (!response.ok) throw new Error(body.error?.message || `HTTP ${response.status}`);
  return body as T;
}

export function StaffPanel({ oem = false }: { oem?: boolean }) {
  const t = useTranslations("staff");
  const locale = useLocale();
  const viewer = useViewer();
  const client = useQueryClient();
  const path = oem ? "/channel/staff" : "/admin/staff";
  const query = useQuery({ queryKey: [path], queryFn: () => request<StaffList>(path) });
  const security = useQuery({ queryKey: [oem ? "/channel/me/2fa" : "/admin/me/2fa"], queryFn: () => request<{ item: { enabled?: boolean; status?: string } }>(oem ? "/channel/me/2fa" : "/admin/me/2fa") });
  const [filter, setFilter] = useState("");
  const [roleFilter, setRoleFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const [operation, setOperation] = useState<Operation | null>(null);
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [roles, setRoles] = useState<string[]>([]);
  const [otp, setOtp] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const historyID = operation?.mode === "history" ? operation.member.user_id : "";
  const history = useQuery({ queryKey: [path, historyID, "history"], enabled: Boolean(historyID), queryFn: () => request<History>(`${path}/${encodeURIComponent(historyID)}/history`) });
  const totp = security.data?.item?.enabled || security.data?.item?.status === "enabled";
  const labelRole = (role: string) => t(`roles.${roleNames[role] || role}`);
  const time = (value: string) => value ? new Date(value).toLocaleString(locale) : "—";
  const items = (query.data?.items ?? []).filter((member) => `${member.email} ${member.display_name}`.toLowerCase().includes(filter.trim().toLowerCase()) && (!roleFilter || member.roles.includes(roleFilter)) && (!statusFilter || member.status === statusFilter));
  function open(next: Operation) {
    setOperation(next); setError(""); setOtp(""); setPassword(""); setEmail(""); setName("");
    setRoles(next.mode === "create" ? [] : next.member.roles);
  }
  function close() { if (!pending) { setOperation(null); setPassword(""); setOtp(""); } }
  async function save() {
    if (!operation || operation.mode === "history" || pending) return;
    setPending(true); setError("");
    try {
      const body = operation.mode === "create" ? { email: email.trim(), display_name: name.trim(), password, roles } : operation.mode === "roles" ? { roles } : { status: operation.mode === "disable" ? "disabled" : "active" };
      const target = operation.mode === "create" ? path : `${path}/${encodeURIComponent(operation.member.user_id)}`;
      await request(target, { method: operation.mode === "create" ? "POST" : "PATCH", headers: { ...confirmHeaders, ...(totp ? { "X-Tokenhub-TOTP": otp } : {}) }, body: JSON.stringify(body) });
      setMessage(t("saved")); setOperation(null); setPassword(""); setOtp("");
      await client.invalidateQueries({ queryKey: [path] });
    } catch (caught) { setError(caught instanceof Error ? caught.message : t("saveError")); }
    finally { setPending(false); }
  }
  const form = operation?.mode === "create" || operation?.mode === "roles";
  const invalid = !operation || operation.mode === "history" || (form && !roles.length) || (operation.mode === "create" && !email.trim()) || (totp && !/^\d{6}$/.test(otp));
  return <div className="flex flex-col gap-6">
    <ConsolePageHeader eyebrow={oem ? "OEM" : "ADMIN"} title={t("title")} description={t(oem ? "oemDescription" : "platformDescription")} actions={<Button disabled={!query.data} onClick={() => open({ mode: "create" })}>{t("create")}</Button>} />
    <section className="rounded-card border border-hairline bg-canvas-raised p-5">
      <div className="mb-5 flex flex-wrap gap-3">
        <Input className="max-w-xs" aria-label={t("search")} placeholder={t("search")} value={filter} onChange={(event) => setFilter(event.target.value)} />
        <select aria-label={t("roleFilter")} className="h-10 rounded-control border border-hairline bg-canvas-raised px-3 text-sm" value={roleFilter} onChange={(event) => setRoleFilter(event.target.value)}><option value="">{t("allRoles")}</option>{query.data?.roles.map((role) => <option key={role} value={role}>{labelRole(role)}</option>)}</select>
        <select aria-label={t("statusFilter")} className="h-10 rounded-control border border-hairline bg-canvas-raised px-3 text-sm" value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}><option value="">{t("allStatuses")}</option><option value="active">{t("active")}</option><option value="disabled">{t("disabled")}</option></select>
        <Button variant="outline" disabled={query.isFetching} onClick={() => void query.refetch()}>{t("refresh")}</Button>
      </div>
      {message ? <p role="status" className="mb-4 text-sm">{message}</p> : null}
      {query.isPending ? <p role="status">{t("loading")}</p> : query.isError ? <p role="alert">{t("loadError")}</p> : <div className="overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr className="border-b border-hairline">{["employee", "role", "status", "updated", "actions"].map((key) => <th key={key} className="whitespace-nowrap px-2 py-3 font-medium text-ink-secondary">{t(key)}</th>)}</tr></thead><tbody>{items.map((member) => <tr key={member.user_id} className="border-b border-hairline last:border-0">
        <td className="max-w-48 break-all px-2 py-4"><p className="font-medium">{member.display_name || member.email}</p>{member.display_name ? <p className="mt-1 text-xs text-ink-secondary">{member.email}</p> : null}</td>
        <td className="whitespace-nowrap px-2 py-4">{member.roles.map(labelRole).join("、")}</td><td className="whitespace-nowrap px-2 py-4">{t(member.status)}</td>
        <td className="max-w-48 break-all px-2 py-4"><p>{member.updated_by_email}</p><p className="mt-1 text-xs text-ink-secondary">{time(member.updated_at)}</p></td>
        <td className="px-2 py-4"><div className="flex flex-wrap gap-2"><Button size="sm" variant="outline" disabled={member.user_id === viewer.userId} onClick={() => open({ mode: "roles", member })}>{t("editRoles")}</Button><Button size="sm" variant="outline" disabled={member.user_id === viewer.userId} onClick={() => open({ mode: member.status === "active" ? "disable" : "enable", member })}>{t(member.status === "active" ? "disable" : "enable")}</Button><Button size="sm" variant="outline" onClick={() => open({ mode: "history", member })}>{t("history")}</Button></div></td>
      </tr>)}</tbody></table>{!items.length ? <p className="py-6 text-sm text-ink-secondary">{t("empty")}</p> : null}</div>}
    </section>
    <Dialog open={Boolean(operation)} onOpenChange={(isOpen) => { if (!isOpen) close(); }}><DialogContent className="max-h-[85vh] overflow-y-auto"><DialogHeader><DialogTitle>{operation ? t(operation.mode === "roles" ? "editRoles" : operation.mode) : ""}</DialogTitle><DialogDescription>{operation?.mode === "create" ? t("createHint") : operation?.mode === "history" ? operation.member.email : `${operation && "member" in operation ? operation.member.email : ""} · ${t(operation?.mode === "disable" ? "disableHint" : operation?.mode === "enable" ? "enableHint" : "rolesHint")}`}</DialogDescription></DialogHeader>
      {operation?.mode === "create" ? <div className="grid gap-4"><label className="grid gap-2 text-sm">{t("email")}<Input type="email" autoComplete="off" value={email} disabled={pending} onChange={(event) => setEmail(event.target.value)} /></label><label className="grid gap-2 text-sm">{t("name")}<Input value={name} disabled={pending} onChange={(event) => setName(event.target.value)} /></label><label className="grid gap-2 text-sm">{t("password")}<Input aria-label={t("password")} type="password" autoComplete="new-password" value={password} disabled={pending} onChange={(event) => setPassword(event.target.value)} /><span className="text-xs text-ink-secondary">{t("passwordHint")}</span></label></div> : null}
      {form ? <fieldset disabled={pending} className="grid gap-3"><legend className="mb-3 text-sm font-medium">{t("role")}</legend>{query.data?.roles.map((role) => <label key={role} className="flex gap-3 rounded-control border border-hairline p-3"><input className="mt-1 size-4 accent-brand" type="checkbox" checked={roles.includes(role)} onChange={(event) => setRoles(event.target.checked ? [...roles, role] : roles.filter((value) => value !== role))} /><span><span className="text-sm font-medium">{labelRole(role)}</span><span className="mt-1 block text-xs text-ink-secondary">{t(`permissions.${oem ? "oem" : "platform"}.${roleNames[role]}`)}</span></span></label>)}</fieldset> : null}
      {operation?.mode === "history" ? <div className="grid gap-4 text-sm"><p>{t("created")} · {operation.member.created_by_email} · {time(operation.member.created_at)}</p>{history.isPending ? <p role="status">{t("loading")}</p> : history.isError ? <p role="alert">{t("loadError")}</p> : history.data?.items.length ? <ul className="divide-y divide-hairline">{history.data.items.map((item) => <li key={item.id} className="py-3"><p className="font-medium">{t(`historyActions.${item.action.replaceAll(".", "_")}`)}</p><p className="mt-1 text-xs text-ink-secondary">{history.data?.actors[item.actor_user_id] || item.actor_user_id} · {time(item.created_at)}</p><p className="mt-2">{item.before ? `${item.before.roles.map(labelRole).join("、")} (${t(item.before.status)}) → ` : ""}{item.after?.roles.map(labelRole).join("、")} {item.after ? `(${t(item.after.status)})` : ""}</p></li>)}</ul> : <p>{t("noHistory")}</p>}</div> : null}
      {totp && operation?.mode !== "history" ? <label className="grid gap-2 text-sm">{t("otp")}<Input inputMode="numeric" autoComplete="one-time-code" value={otp} disabled={pending} onChange={(event) => setOtp(event.target.value)} maxLength={6} /></label> : null}
      {security.isError && operation?.mode !== "history" ? <p role="alert">{t("loadError")}</p> : null}
      {error ? <p role="alert" className="text-sm text-danger">{error}</p> : null}
      <DialogFooter><Button variant="outline" disabled={pending} onClick={close}>{t(operation?.mode === "history" ? "close" : "cancel")}</Button>{operation?.mode !== "history" ? <Button disabled={pending || invalid || security.isPending || security.isError} onClick={() => void save()}>{pending ? t("saving") : t(operation?.mode === "create" ? "create" : operation?.mode === "roles" ? "save" : operation?.mode || "save")}</Button> : null}</DialogFooter>
    </DialogContent></Dialog>
  </div>;
}
