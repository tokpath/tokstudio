"use client";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import { ConfirmDialog } from "@/components/confirm-button";
import { ActionRow, LeadActions } from "@/components/console/action-row";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { ModelAllowlistPicker } from "@/components/console/model-allowlist-picker";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { ScrollTable } from "@/components/ui/scroll-table";
import { ListResourceView } from "@/components/console/list-resource-view";
import { SubmitStatus } from "@/components/console/submit-status";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import type { CatalogModel } from "@/lib/catalog";
import { optionalPositiveInt } from "@/lib/key-limits";
import { fetchKeyPages } from "@/lib/key-resources";
import { useViewer } from "@/components/rbac/viewer-context";
import { keysCreateQueryOpen } from "@/lib/overview-guide";
import { statusLabelKey } from "@/lib/status-copy";
import { copyText, errorMessageFromBody, readResponseBody } from "@/lib/submit-result";
import { useToast } from "@/lib/toast";
import { keyRemainingMinor, keyState, keyDocsHref, preferredKeyModel, usdToMinor, type KeyPolicy } from "@/lib/key-policy";
export { optionalPositiveInt, parseAllowlist } from "@/lib/key-limits";
export type APIKeyItem = KeyPolicy & {
    id: string;
    name: string;
    prefix: string;
    key?: string;
    status: string;
    rpm_limit?: number;
    concurrency_limit?: number;
    allowlist?: string[];
    expires_at?: string | null;
    last_used_at?: string | null;
    created_at?: string;
};
/** 列表默认只露前缀；完整密文要用户点「显示」或「复制」才出来。 */
export function maskAPIKey(prefix: string, secret?: string, revealed = false): string {
    if (revealed && secret) {
        return secret;
    }
    const rest = secret && secret.length > prefix.length ? secret.length - prefix.length : 8;
    return `${prefix}${"•".repeat(Math.min(12, Math.max(rest, 8)))}`;
}
export function formatWhen(value?: string | null, empty = "—"): string {
    if (!value) {
        return empty;
    }
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) {
        return empty;
    }
    return parsed.toLocaleString();
}
function statusTone(item: APIKeyItem): "success" | "warn" | "neutral" {
    if (item.status === "disabled") {
        return "warn";
    }
    if (item.expires_at && new Date(item.expires_at) <= new Date()) {
        return "warn";
    }
    if (item.status === "active") {
        return "success";
    }
    return "neutral";
}
type KeyAct = "rotate" | "disable" | "expire";
type KeyActHandler = (id: string) => boolean | void | Promise<boolean | void>;
type KeysListProps = {
    items: APIKeyItem[];
    revealedIds?: string[];
    onCopy?: (id: string) => void;
    onToggleReveal?: (id: string) => void;
    onRotate?: KeyActHandler;
    onDisable?: KeyActHandler;
    onExpire?: KeyActHandler;
    onEdit?: (item: APIKeyItem) => void;
    onEnable?: KeyActHandler;
    actionError?: string;
    onActionErrorClear?: () => void;
    onConfirmationChange?: (open: boolean) => void;
};
function KeyMoreMenu({ item, onPick, canRotate, canDisable, canExpire, }: {
    item: APIKeyItem;
    onPick: (item: APIKeyItem, action: KeyAct) => void;
    canRotate: boolean;
    canDisable: boolean;
    canExpire: boolean;
}) {
    const t = useTranslations("user");
    const [open, setOpen] = useState(false);
    const rootRef = useRef<HTMLDivElement>(null);
    useEffect(() => {
        if (!open) {
            return;
        }
        function onDoc(event: MouseEvent) {
            if (!rootRef.current?.contains(event.target as Node)) {
                setOpen(false);
            }
        }
        function onKey(event: KeyboardEvent) {
            if (event.key === "Escape") {
                setOpen(false);
            }
        }
        document.addEventListener("mousedown", onDoc);
        document.addEventListener("keydown", onKey);
        return () => {
            document.removeEventListener("mousedown", onDoc);
            document.removeEventListener("keydown", onKey);
        };
    }, [open]);
    if (!canRotate && !canDisable && !canExpire) {
        return null;
    }
    function pick(action: KeyAct) {
        setOpen(false);
        onPick(item, action);
    }
    return (<div ref={rootRef} className="relative">
      <Button type="button" size="sm" variant="outline" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen((current) => !current)}>
        {t("moreActions")}
      </Button>
      {open ? (<div role="menu" className="absolute right-0 z-20 mt-1.5 min-w-[10rem] rounded-card border border-hairline bg-canvas-raised p-1.5 shadow-[0_1px_2px_rgba(20,20,20,0.06)]">
          {canRotate ? (<Button type="button" role="menuitem" size="sm" variant="ghost" className="w-full justify-start" onClick={() => pick("rotate")}>
              {t("rotate")}
            </Button>) : null}
          {canDisable ? (<Button type="button" role="menuitem" size="sm" variant="ghost" className="w-full justify-start" onClick={() => pick("disable")}>
              {t("disable")}
            </Button>) : null}
          {canExpire ? (<Button type="button" role="menuitem" size="sm" variant="ghost" className="w-full justify-start" onClick={() => pick("expire")}>
              {t("expireNow")}
            </Button>) : null}
        </div>) : null}
    </div>);
}
function KeyStatusBadge({ item }: {
    item: APIKeyItem;
}) {
    const tc = useTranslations("common");
    const tx = useTranslations("keyUX");
    const state = keyState(item);
    if (state !== "active" && state !== "disabled")
        return <Badge tone="warn">{tx(state)}</Badge>;
    const key = statusLabelKey(item.status);
    return <Badge tone={statusTone(item)}>{key ? tc(key) : item.status ? tc("stUnknown", { status: item.status }) : "—"}</Badge>;
}
function KeySecretActions({ item, revealed, onCopy, onToggleReveal, }: {
    item: APIKeyItem;
    revealed: boolean;
    onCopy?: (id: string) => void;
    onToggleReveal?: (id: string) => void;
}) {
    const t = useTranslations("user");
    const tc = useTranslations("common");
    return (<div className="flex min-w-0 max-w-md flex-col gap-2">
      <code className="th-code block overflow-x-auto whitespace-nowrap text-[13px] text-ink">
        {maskAPIKey(item.prefix, item.key, revealed)}
      </code>
      <ActionRow>
        {onCopy ? (<Button size="sm" variant="outline" onClick={() => onCopy(item.id)}>
            {tc("copy")}
          </Button>) : null}
        {onToggleReveal ? (<Button size="sm" variant="ghost" onClick={() => onToggleReveal(item.id)}>
            {revealed ? t("hide") : t("reveal")}
          </Button>) : null}
      </ActionRow>
    </div>);
}
export function KeysList({ items, revealedIds = [], onCopy, onToggleReveal, onRotate, onDisable, onExpire, onEdit, onEnable, actionError, onActionErrorClear, onConfirmationChange, }: KeysListProps) {
    const t = useTranslations("user");
    const tc = useTranslations("common");
    const tx = useTranslations("keyUX");
    const [pending, setPending] = useState<{
        item: APIKeyItem;
        action: KeyAct;
    } | null>(null);
    useEffect(() => { onConfirmationChange?.(pending !== null); }, [pending, onConfirmationChange]);
    const copy = {
        rotate: { title: t("rotateConfirmTitle"), description: t("rotateConfirm") },
        disable: { title: t("disableConfirmTitle"), description: t("disableConfirm") },
        expire: { title: t("expireConfirmTitle"), description: t("expireConfirm") },
    };
    function pick(item: APIKeyItem, action: KeyAct) {
        onActionErrorClear?.();
        setPending({ item, action });
    }
    async function confirmPending() {
        if (!pending) {
            return false;
        }
        const run = pending.action === "rotate" ? onRotate : pending.action === "disable" ? onDisable : onExpire;
        return (await run?.(pending.item.id)) === true;
    }
    const menu = (item: APIKeyItem) => (<ActionRow>
 <Button size="sm" variant="outline" asChild><Link href={keyDocsHref(item.id, preferredKeyModel(item))}>{tx("instructions")}</Link></Button>
 {onEdit ? <Button size="sm" variant="outline" onClick={() => onEdit(item)}>{tx("edit")}</Button> : null}
 {item.status === "disabled" && onEnable ? <Button size="sm" variant="outline" onClick={() => void onEnable(item.id)}>{tx("enable")}</Button> : null}
    <KeyMoreMenu item={item} onPick={pick} canRotate={Boolean(onRotate)} canDisable={Boolean(onDisable) && item.status !== "disabled"} canExpire={Boolean(onExpire)}/>
 </ActionRow>);
    if (items.length === 0) {
        return <EmptyLedger title={t("emptyKeys")} detail={t("emptyKeysDetail")}/>;
    }
    const revealed = new Set(revealedIds);
    return (<>
      <ul data-testid="key-cards" className="flex flex-col gap-3 xl:hidden">
        {items.map((item) => {
            const isRevealed = revealed.has(item.id);
            return (<li key={item.id} className="rounded-card border border-hairline bg-canvas-raised p-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="font-medium text-ink">{item.name}</p>
                  <p className="mt-1 font-mono text-xs text-ink-mute">{item.prefix}</p>
                </div>
                <KeyStatusBadge item={item}/>
              </div>
              <div className="mt-3"><KeyBudgetSummary item={item}/><p className="mt-2 text-xs">{tx("models")}: {item.model_mode === "all" ? tx("all") : item.allowlist?.join(", ")} · {tx("expiry")}: {item.expires_at ? formatWhen(item.expires_at) : tx("permanent")}</p>{menu(item)}</div>
              <details className="mt-3">
                <summary className="cursor-pointer text-sm text-brand-emphasis">{t("expandKeyDetails")}</summary>
                <div className="mt-3 flex flex-col gap-3">
                  <KeySecretActions item={item} revealed={isRevealed} onCopy={onCopy} onToggleReveal={onToggleReveal}/>
                  <p className="text-sm text-ink">
                    {item.rpm_limit ? `${t("rpm")} ${item.rpm_limit}` : ""}
                    {item.concurrency_limit ? t("concurrency", { n: item.concurrency_limit }) : ""}
                  </p>
                  <p className="text-sm text-ink-secondary">
                    {t("allowlistLine", { list: item.allowlist?.length ? item.allowlist.join(", ") : tc("unlimited") })}
                  </p>
                  <p className="text-xs text-ink-mute">{formatWhen(item.last_used_at, t("neverUsed"))}</p>

                </div>
              </details>
            </li>);
        })}
      </ul>
      <ScrollTable density="ledger" className="hidden rounded-card border border-hairline xl:block" minWidthClassName="min-w-[52rem]" getRowId={(item) => item.id} rows={items} columns={[
            {
                id: "name",
                header: t("colName"),
                cell: (item) => <span className="text-ink">{item.name}</span>,
            },
            {
                id: "key",
                header: t("colKey"),
                cell: (item) => (<KeySecretActions item={item} revealed={revealed.has(item.id)} onCopy={onCopy} onToggleReveal={onToggleReveal}/>),
            },
            {
                id: "status",
                header: t("colStatus"),
                cell: (item) => <KeyStatusBadge item={item}/>,
            },
            {
                id: "limits",
                header: tx("budget"),
                cell: (item) => <KeyBudgetSummary item={item}/>,
            },
            {
                id: "allowlist",
                header: t("allowTitle"),
                cell: (item) => (<span className="text-ink-secondary">
                {t("allowlistLine", { list: item.allowlist?.length ? item.allowlist.join(", ") : tc("unlimited") })}
              </span>),
            },
            {
                id: "expiry",
                header: tx("expiry"),
                cell: item => <span>{item.expires_at ? formatWhen(item.expires_at) : tx("permanent")}</span>,
            },
            {
                id: "lastUsed",
                header: t("colLastUsed"),
                cell: (item) => <span className="text-ink-mute">{formatWhen(item.last_used_at, t("neverUsed"))}</span>,
            },
            {
                id: "actions",
                header: t("colActions"),
                cell: (item) => menu(item),
            },
        ]}/>
      <ConfirmDialog open={pending !== null} onOpenChange={(open) => {
            if (!open) {
                setPending(null);
                onActionErrorClear?.();
            }
        }} title={pending ? copy[pending.action].title : ""} description={pending ? copy[pending.action].description : undefined} error={actionError} onConfirm={confirmPending}/>
    </>);
}
type BudgetPeriod = "lifetime" | "month" | "quarter" | "year";
type KeyForm = {
    name: string;
    model_mode: "all" | "selected";
    allowlist: string[];
    budget: string;
    limited: boolean;
    period: BudgetPeriod;
    expiry: string;
    expiring: boolean;
    rpm: string;
    concurrency: string;
};
const defaultForm: KeyForm = { name: "", model_mode: "all", allowlist: [], budget: "", limited: false, period: "lifetime", expiry: "", expiring: false, rpm: "", concurrency: "" };
function budgetPeriodOf(value?: string): BudgetPeriod {
    return value === "month" || value === "quarter" || value === "year" ? value : "lifetime";
}
function localDateTime(value?: string | null) { if (!value)
    return ""; const d = new Date(value); return new Date(d.getTime() - d.getTimezoneOffset() * 60000).toISOString().slice(0, 16); }
export default function KeysPanel() {
    const t = useTranslations("user");
    const tc = useTranslations("common");
    const tx = useTranslations("keyUX");
    const viewer = useViewer();
    const scope = `${viewer.userId ?? ""}:${typeof window !== "undefined" ? window.location.host : ""}`;
    const list = useListResource<APIKeyItem>({ queryKey: scope, load: () => fetchKeyPages(`${apiBase}/v1/me/api-keys`) });
    const [open, setOpen] = useState(false);
    const [edit, setEdit] = useState<APIKeyItem | null>(null);
    const [created, setCreated] = useState<APIKeyItem | null>(null);
    const [form, setForm] = useState<KeyForm>(defaultForm);
    const [models, setModels] = useState<CatalogModel[]>([]);
    const [modelsError, setModelsError] = useState("");
    const [contextModel, setContextModel] = useState("");
    const [modelsLoading, setModelsLoading] = useState(true);
    const [modelRevision, setModelRevision] = useState(0);
    const [unknownCreate, setUnknownCreate] = useState(false);
    const [confirmationOpen, setConfirmationOpen] = useState(false);
    const createOperation = useRef({ id: "", body: "" });
    const actionSession = useRef(0);
    const scopeRef = useRef(scope);
    scopeRef.current = scope;
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState("");
    const [fallback, setFallback] = useState("");
    const [revealed, setRevealed] = useState<string[]>([]);
    const session = useRef(0);
    const message = useToast(s => s.message);
    const setMessage = useToast(s => s.setMessage);
    function openForm(item?: APIKeyItem) { session.current++; createOperation.current = { id: crypto.randomUUID(), body: "" }; setUnknownCreate(false); setEdit(item ?? null); setCreated(null); setError(""); setFallback(""); setBusy(false); setOpen(true); setForm(item ? { name: item.name, model_mode: item.model_mode ?? (item.allowlist?.length ? "selected" : "all"), allowlist: item.allowlist ?? [], limited: item.budget_limit_minor != null, budget: item.budget_limit_minor != null ? String(item.budget_limit_minor / 1e6) : "", period: budgetPeriodOf(item.budget_period), expiry: localDateTime(item.expires_at), expiring: !!item.expires_at, rpm: String(item.rpm_limit ?? ""), concurrency: String(item.concurrency_limit ?? "") } : defaultForm); }
    function close() { session.current++; setOpen(false); setError(""); setBusy(false); }
    useEffect(() => { session.current++; actionSession.current++; setRevealed([]); setUnknownCreate(false); setOpen(false); setCreated(null); setError(""); setFallback(""); }, [scope]);
    useEffect(() => { const query = new URLSearchParams(window.location.search); setContextModel(query.get("model") ?? ""); if (keysCreateQueryOpen(window.location.search))
        openForm(); }, [scope]);
    useEffect(() => { let cancelled = false; const controller = new AbortController(); setModels([]); setModelsError(""); setModelsLoading(true); void fetchKeyPages<CatalogModel>(`${apiBase}/v1/public/models`, controller.signal).then(result => { if (cancelled)
        return; setModelsLoading(false); if (!result.ok) {
        setModelsError(result.message || tx("modelsFailed"));
        return;
    } setModels(result.items ?? []); }); return () => { cancelled = true; controller.abort(); }; }, [scope, tx, modelRevision]);
    const editQuerySeen = useRef("");
    useEffect(() => { const id = new URLSearchParams(window.location.search).get("edit"); if (!id || editQuerySeen.current === `${scope}:${id}`)
        return; const found = list.snapshot.items.find(item => item.id === id); if (found) {
        editQuerySeen.current = `${scope}:${id}`;
        openForm(found);
    } }, [list.snapshot.items, scope]);
    const update = <K extends keyof KeyForm>(key: K, value: KeyForm[K]) => setForm(current => ({ ...current, [key]: value }));
    async function save(event: React.FormEvent) {
        event.preventDefault();
        const active = session.current;
        if (busy)
            return;
        const amount = form.limited ? usdToMinor(form.budget) : null;
        const rpm = optionalPositiveInt(form.rpm);
        const concurrency = optionalPositiveInt(form.concurrency);
        if (!unknownCreate && (!form.name.trim() || (form.model_mode === "selected" && form.allowlist.length === 0) || (form.limited && amount === null) || (form.expiring && (!form.expiry || !Number.isFinite(Date.parse(form.expiry)))) || rpm === "invalid" || concurrency === "invalid")) {
            setError(tx("invalid"));
            return;
        }
        setBusy(true);
        setError("");
        try {
            const payload = JSON.stringify({ name: form.name.trim(), model_mode: form.model_mode, allowlist: form.model_mode === "selected" ? form.allowlist : [], budget_limit_minor: amount, budget_period: form.limited ? form.period : "lifetime", expires_at: form.expiring ? new Date(form.expiry).toISOString() : null, rpm_limit: typeof rpm === "number" ? rpm : 0, concurrency_limit: typeof concurrency === "number" ? concurrency : 0, ...(!edit ? { operation_id: createOperation.current.id } : {}) });
            if (!edit && !unknownCreate)
                createOperation.current.body = payload;
            const res = await fetch(`${apiBase}/v1/me/api-keys${edit ? `/${encodeURIComponent(edit.id)}/limits` : ""}`, { method: edit ? "PUT" : "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: !edit ? createOperation.current.body : payload });
            const body = await readResponseBody(res);
            if (active !== session.current) {
                if (res.ok)
                    void list.reload();
                return;
            }
            if (!res.ok) {
                setError(errorMessageFromBody(body, tc("createFailed")));
                return;
            }
            const item = (body as {
                item: APIKeyItem;
            }).item;
            if (!item?.id)
                throw new Error("unknown creation response");
            if (edit) {
                close();
                setMessage(tx("saved"));
            }
            else {
                setCreated(item);
                setUnknownCreate(false);
            }
            void list.reload();
        }
        catch {
            if (active === session.current) {
                setError(edit ? tc("listNetwork") : tx("createUnknown"));
                if (!edit)
                    setUnknownCreate(true);
                void list.reload();
            }
        }
        finally {
            if (active === session.current)
                setBusy(false);
        }
    }
    async function copy(id: string, secret?: string) {
        const active = ++actionSession.current;
        const actionScope = scope;
        const value = secret ?? list.snapshot.items.find(item => item.id === id)?.key;
        if (!value) {
            setMessage(t("copyMissing"));
            return;
        }
        try {
            const res = await fetch(`${apiBase}/v1/me/api-keys/${encodeURIComponent(id)}/copy`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: "{}" });
            const body = await readResponseBody(res);
            if (active !== actionSession.current || actionScope !== scopeRef.current)
                return false;
            if (!res.ok) {
                setError(errorMessageFromBody(body, t("actFail")));
                return;
            }
            const copied = await copyText(value);
            if (active !== actionSession.current || actionScope !== scopeRef.current)
                return;
            if (copied) {
                setFallback("");
                setMessage(t("copiedAudit"));
            }
            else {
                setFallback(value);
                setError(t("copyFailed"));
            }
        }
        catch {
            if (active === actionSession.current && actionScope === scopeRef.current)
                setError(tc("listNetwork"));
        }
    }
    async function act(id: string, action: KeyAct | "enable") { const active = ++actionSession.current; const actionScope = scope; try {
        const res = await fetch(`${apiBase}/v1/me/api-keys/${encodeURIComponent(id)}/${action}`, { method: "POST", credentials: "include", headers: { "Content-Type": "application/json" }, body: action === "expire" ? JSON.stringify({ expires_at: new Date().toISOString() }) : "{}" });
        const body = await readResponseBody(res);
        if (active !== actionSession.current || actionScope !== scopeRef.current)
            return false;
        if (!res.ok) {
            setError(errorMessageFromBody(body, t("actFail")));
            return false;
        }
        setError("");
        setMessage(tx("saved"));
        await list.reload();
        return true;
    }
    catch {
        if (active === actionSession.current && actionScope === scopeRef.current)
            setError(tc("listNetwork"));
        return false;
    } }
    return <Card>
  <LeadActions lead={<p className="text-sm text-ink-secondary">{tx("lead")}</p>} actions={<><Button variant="outline" onClick={() => void list.reload()}>{tc("refresh")}</Button><Button onClick={() => openForm()}>{t("createKey")}</Button></>}/>
  <ListResourceView snapshot={list.snapshot} emptyTitle={t("emptyKeys")} emptyDetail={tx("empty")} loadingTitle={t("keysTitle")} onRetry={() => void list.reload()} name="keys" emptyAction={<Button onClick={() => openForm()}>{t("createKey")}</Button>}>
   <KeysList items={list.snapshot.items} revealedIds={revealed} onCopy={id => void copy(id)} onToggleReveal={id => setRevealed(current => current.includes(id) ? current.filter(v => v !== id) : [...current, id])} onEdit={openForm} onEnable={id => act(id, "enable")} onRotate={id => act(id, "rotate")} onDisable={id => act(id, "disable")} onExpire={id => act(id, "expire")} actionError={error} onActionErrorClear={() => setError("")} onConfirmationChange={setConfirmationOpen}/>
  </ListResourceView>
  {message ? <p role="status" className="mt-3 text-sm text-ink-secondary">{message}</p> : null}
  {!open && !confirmationOpen && (error || fallback) ? <SubmitStatus error={error} selectable={fallback}/> : null}
  <Dialog open={open} onOpenChange={value => { if (!value)
        close(); }}><DialogContent className="max-h-[90vh] overflow-y-auto">
  <DialogHeader><DialogTitle>{created ? t("createdTitle") : edit ? tx("edit") : t("createKey")}</DialogTitle><DialogDescription>{created ? tx("createdLead") : tx("formLead")}</DialogDescription></DialogHeader>
  {created ? <>
   <KeyBudgetSummary item={created}/>
   <code data-testid="key-secret" className="th-code block break-all p-3">{created.key ?? maskAPIKey(created.prefix)}</code>
   <Button variant="outline" onClick={() => void copy(created.id, created.key)}>{tc("copy")}</Button>
   <ActionRow><Button variant="outline" asChild><Link href={keyDocsHref(created.id, preferredKeyModel(created, contextModel), "agent")}>{tx("agent")}</Link></Button><Button variant="outline" asChild><Link href={keyDocsHref(created.id, preferredKeyModel(created, contextModel))}>{tx("protocol")}</Link></Button></ActionRow>
   <SubmitStatus error={error} selectable={fallback}/><DialogFooter><Button onClick={close}>{t("done")}</Button></DialogFooter>
  </> : <form onSubmit={save} className="grid gap-4"><fieldset disabled={unknownCreate || busy} className="grid gap-4">
   <label className="grid gap-1 text-sm">{t("nameLabel")}<input className="h-10 rounded-control border border-hairline bg-canvas px-3" value={form.name} onChange={e => update("name", e.target.value)} placeholder={t("namePh")}/></label>
   <fieldset className="grid gap-2"><legend className="mb-2 text-sm font-medium">{tx("models")}</legend>
    <label className="text-sm"><input type="radio" name="model-mode" checked={form.model_mode === "all"} onChange={() => update("model_mode", "all")}/> {tx("all")}</label>
    <label className="text-sm"><input type="radio" name="model-mode" checked={form.model_mode === "selected"} onChange={() => update("model_mode", "selected")}/> {tx("selected")}</label>
    {form.model_mode === "selected" ? <ModelAllowlistPicker value={form.allowlist} onChange={value => update("allowlist", value)} options={models} allLabel={tx("selectAtLeastOne")} searchLabel={t("modelSearch")} selectedLabel={tx("models")}/> : <p className="text-xs text-ink-secondary">{modelsLoading ? tx("modelsLoading") : modelsError ? "" : tx("modelCount", { count: models.length })}</p>}
    {modelsError ? <div role="alert" className="text-sm text-warning">{modelsError} <Button type="button" variant="outline" onClick={() => setModelRevision(v => v + 1)}>{tx("reload")}</Button></div> : null}
   </fieldset>
   <fieldset className="grid gap-2"><legend className="mb-2 text-sm font-medium">{tx("budget")}</legend>
    <label className="text-sm"><input type="radio" name="budget-mode" checked={!form.limited} onChange={() => update("limited", false)}/> {tx("unlimited")}</label>
    <label className="text-sm"><input type="radio" name="budget-mode" checked={form.limited} onChange={() => update("limited", true)}/> {tx("limited")}</label>
    {form.limited ? <label className="grid gap-1 text-sm">USD<input type="text" inputMode="decimal" className="h-10 rounded-control border border-hairline bg-canvas px-3" value={form.budget} onChange={e => update("budget", e.target.value)}/></label> : null}
    {form.limited ? <fieldset className="grid gap-2"><legend className="text-sm font-medium">{tx("period")}</legend>
     {(["lifetime", "month", "quarter", "year"] as const).map(period => <label key={period} className="text-sm"><input type="radio" name="budget-period" checked={form.period === period} onChange={() => update("period", period)}/> {tx(period === "lifetime" ? "periodLifetime" : period === "month" ? "periodMonth" : period === "quarter" ? "periodQuarter" : "periodYear")}</label>)}
    </fieldset> : null}
    {form.limited ? <div className="space-y-1 text-xs text-warning"><p>{tx("finiteCompatibility")}</p>{modelsLoading ? <p>{tx("modelsLoading")}</p> : modelsError ? <p>{tx("modelsFailed")}</p> : <p>{tx("finiteModels", { models: models.filter(m => m.capabilities?.budget_control_supported === true || m.capabilities?.text_budget_control_supported === true).filter(m => form.model_mode === "all" || form.allowlist.includes(m.id)).map(m => m.id).join(", ") || tx("noneBudgetable") })}</p>}</div> : null}
    {edit ? <KeyBudgetSummary item={edit}/> : null}<p className="text-xs text-ink-secondary">{tx("budgetHint")}</p>
   </fieldset>
   <fieldset className="grid gap-2"><legend className="mb-2 text-sm font-medium">{tx("expiry")}</legend>
    <label className="text-sm"><input type="radio" name="expiry-mode" checked={!form.expiring} onChange={() => update("expiring", false)}/> {tx("permanent")}</label>
    <label className="text-sm"><input type="radio" name="expiry-mode" checked={form.expiring} onChange={() => update("expiring", true)}/> {tx("expires")}</label>
    {form.expiring ? <label className="grid gap-1 text-sm">{tx("localTime", { zone: Intl.DateTimeFormat().resolvedOptions().timeZone })}<input type="datetime-local" className="h-10 rounded-control border border-hairline bg-canvas px-3" value={form.expiry} onChange={e => update("expiry", e.target.value)}/></label> : null}
   </fieldset>
   <details><summary className="cursor-pointer text-sm text-brand">{t("advanced")}</summary><div className="mt-3 grid gap-3">{(["rpm", "concurrency"] as const).map(field => <label key={field} className="grid gap-1 text-sm">{field === "rpm" ? t("rpm") : t("conc")}<input className="h-10 rounded-control border border-hairline bg-canvas px-3" inputMode="numeric" value={form[field]} onChange={e => update(field, e.target.value)}/></label>)}</div></details>
   </fieldset><SubmitStatus error={error}/><DialogFooter><Button type="button" variant="outline" onClick={close}>{tc("cancel")}</Button><Button type="submit" disabled={busy}>{busy ? tc("submitting") : edit ? tx("save") : unknownCreate ? tx("recoverCreate") : tc("create")}</Button></DialogFooter>
  </form>}
  </DialogContent></Dialog>
 </Card>;
}
function KeyBudgetSummary({ item }: {
    item: APIKeyItem;
}) {
    const tx = useTranslations("keyUX");
    const used = item.budget_used_minor ?? 0;
    const reserved = item.budget_reserved_minor ?? 0;
    const limit = item.budget_limit_minor;
    const remaining = keyRemainingMinor(item);
    const period = item.budget_period === "month" ? tx("periodMonth") : item.budget_period === "quarter" ? tx("periodQuarter") : item.budget_period === "year" ? tx("periodYear") : item.budget_period === "lifetime" ? tx("periodLifetime") : "";
    return <div className="space-y-1 text-xs text-ink-secondary"><p>{tx("used", { amount: (used / 1e6).toFixed(6) })} · {limit == null ? tx("unlimited") : tx("limit", { amount: limit / 1e6 })}{period ? ` · ${period}` : ""}</p>{reserved > 0 ? <p>{tx("reserved", { amount: (reserved / 1e6).toFixed(6) })}</p> : null}{remaining != null ? <p>{tx("remaining", { amount: (remaining / 1e6).toFixed(6) })}</p> : null}</div>;
}
