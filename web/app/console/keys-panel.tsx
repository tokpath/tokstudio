"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslations } from "next-intl";
import Link from "next/link";
import { z } from "zod";
import { ConfirmDialog } from "@/components/confirm-button";
import { ActionRow, LeadActions } from "@/components/console/action-row";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { ModelAllowlistPicker } from "@/components/console/model-allowlist-picker";
import { TextField } from "@/components/text-field";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Form, FormField, FormItem, FormLabel, FormMessage } from "@/components/ui/form";
import { ScrollTable } from "@/components/ui/scroll-table";
import { ListResourceView } from "@/components/console/list-resource-view";
import { SubmitStatus } from "@/components/console/submit-status";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import type { CatalogModel } from "@/lib/catalog";
import { optionalPositiveInt } from "@/lib/key-limits";
import { fetchListItems } from "@/lib/list-resource";
import { keyExampleFor, keyVerifyRequest } from "@/lib/key-example";
import { useModelHref } from "@/lib/model-use";
import { keysCreateQueryOpen } from "@/lib/overview-guide";
import { statusLabelKey } from "@/lib/status-copy";
import { copyText, errorMessageFromBody, readResponseBody } from "@/lib/submit-result";
import { useToast } from "@/lib/toast";

export { optionalPositiveInt, parseAllowlist } from "@/lib/key-limits";

export type APIKeyItem = {
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
  actionError?: string;
  onActionErrorClear?: () => void;
};

function KeyMoreMenu({
  item,
  onPick,
  canRotate,
  canDisable,
  canExpire,
}: {
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

  return (
    <div ref={rootRef} className="relative">
      <Button
        type="button"
        size="sm"
        variant="outline"
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
      >
        {t("moreActions")}
      </Button>
      {open ? (
        <div role="menu" className="absolute right-0 z-20 mt-1.5 min-w-[10rem] rounded-card border border-hairline bg-canvas-raised p-1.5 shadow-[0_1px_2px_rgba(20,20,20,0.06)]">
          {canRotate ? (
            <Button type="button" role="menuitem" size="sm" variant="ghost" className="w-full justify-start" onClick={() => pick("rotate")}>
              {t("rotate")}
            </Button>
          ) : null}
          {canDisable ? (
            <Button type="button" role="menuitem" size="sm" variant="ghost" className="w-full justify-start" onClick={() => pick("disable")}>
              {t("disable")}
            </Button>
          ) : null}
          {canExpire ? (
            <Button type="button" role="menuitem" size="sm" variant="ghost" className="w-full justify-start" onClick={() => pick("expire")}>
              {t("expireNow")}
            </Button>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

function KeyStatusBadge({ item }: { item: APIKeyItem }) {
  const tc = useTranslations("common");
  const key = statusLabelKey(item.status);
  return <Badge tone={statusTone(item)}>{key ? tc(key) : item.status ? tc("stUnknown", { status: item.status }) : "—"}</Badge>;
}

function KeySecretActions({
  item,
  revealed,
  onCopy,
  onToggleReveal,
}: {
  item: APIKeyItem;
  revealed: boolean;
  onCopy?: (id: string) => void;
  onToggleReveal?: (id: string) => void;
}) {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  return (
    <div className="flex min-w-0 max-w-md flex-col gap-2">
      <code className="th-code block overflow-x-auto whitespace-nowrap text-[13px] text-ink">
        {maskAPIKey(item.prefix, item.key, revealed)}
      </code>
      <ActionRow>
        {onCopy ? (
          <Button size="sm" variant="outline" onClick={() => onCopy(item.id)}>
            {tc("copy")}
          </Button>
        ) : null}
        {onToggleReveal ? (
          <Button size="sm" variant="ghost" onClick={() => onToggleReveal(item.id)}>
            {revealed ? t("hide") : t("reveal")}
          </Button>
        ) : null}
      </ActionRow>
    </div>
  );
}

export function KeysList({
  items,
  revealedIds = [],
  onCopy,
  onToggleReveal,
  onRotate,
  onDisable,
  onExpire,
  actionError,
  onActionErrorClear,
}: KeysListProps) {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const [pending, setPending] = useState<{ item: APIKeyItem; action: KeyAct } | null>(null);
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

  const menu = (item: APIKeyItem) => (
    <KeyMoreMenu
      item={item}
      onPick={pick}
      canRotate={Boolean(onRotate)}
      canDisable={Boolean(onDisable)}
      canExpire={Boolean(onExpire)}
    />
  );

  if (items.length === 0) {
    return <EmptyLedger title={t("emptyKeys")} detail={t("emptyKeysDetail")} />;
  }
  const revealed = new Set(revealedIds);
  return (
    <>
      <ul className="flex flex-col gap-3 md:hidden">
        {items.map((item) => {
          const isRevealed = revealed.has(item.id);
          return (
            <li key={item.id} className="rounded-card border border-hairline bg-canvas-raised p-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="font-medium text-ink">{item.name}</p>
                  <p className="mt-1 font-mono text-xs text-ink-mute">{item.prefix}</p>
                </div>
                <KeyStatusBadge item={item} />
              </div>
              <details className="mt-3">
                <summary className="cursor-pointer text-sm text-brand-emphasis">{t("expandKeyDetails")}</summary>
                <div className="mt-3 flex flex-col gap-3">
                  <KeySecretActions item={item} revealed={isRevealed} onCopy={onCopy} onToggleReveal={onToggleReveal} />
                  <p className="text-sm text-ink">
                    {item.rpm_limit ? `${t("rpm")} ${item.rpm_limit}` : ""}
                    {item.concurrency_limit ? t("concurrency", { n: item.concurrency_limit }) : ""}
                  </p>
                  <p className="text-sm text-ink-secondary">
                    {t("allowlistLine", { list: item.allowlist?.length ? item.allowlist.join(", ") : tc("unlimited") })}
                  </p>
                  <p className="text-xs text-ink-mute">{formatWhen(item.last_used_at, t("neverUsed"))}</p>
                  {menu(item)}
                </div>
              </details>
            </li>
          );
        })}
      </ul>
      <ScrollTable
        density="ledger"
        className="hidden rounded-card border border-hairline md:block"
        minWidthClassName="min-w-[52rem]"
        getRowId={(item) => item.id}
        rows={items}
        columns={[
          {
            id: "name",
            header: t("colName"),
            cell: (item) => <span className="text-ink">{item.name}</span>,
          },
          {
            id: "key",
            header: t("colKey"),
            cell: (item) => (
              <KeySecretActions
                item={item}
                revealed={revealed.has(item.id)}
                onCopy={onCopy}
                onToggleReveal={onToggleReveal}
              />
            ),
          },
          {
            id: "status",
            header: t("colStatus"),
            cell: (item) => <KeyStatusBadge item={item} />,
          },
          {
            id: "limits",
            header: t("colLimits"),
            cell: (item) => (
              <span className="text-ink">
                {item.rpm_limit ? `${t("rpm")} ${item.rpm_limit}` : ""}
                {item.concurrency_limit ? t("concurrency", { n: item.concurrency_limit }) : ""}
              </span>
            ),
          },
          {
            id: "allowlist",
            header: t("allowTitle"),
            cell: (item) => (
              <span className="text-ink-secondary">
                {t("allowlistLine", { list: item.allowlist?.length ? item.allowlist.join(", ") : tc("unlimited") })}
              </span>
            ),
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
        ]}
      />
      <ConfirmDialog
        open={pending !== null}
        onOpenChange={(open) => {
          if (!open) {
            setPending(null);
            onActionErrorClear?.();
          }
        }}
        title={pending ? copy[pending.action].title : ""}
        description={pending ? copy[pending.action].description : undefined}
        error={actionError}
        onConfirm={confirmPending}
      />
    </>
  );
}

const defaultForm = { name: "", allowlist: [] as string[], rpm: "", concurrency: "" };

export default function KeysPanel() {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const list = useListResource<APIKeyItem>({
    load: () => fetchListItems(`${apiBase}/v1/me/api-keys`),
  });
  const items = list.snapshot.items;
  const [createOpen, setCreateOpen] = useState(false);
  const [createdKey, setCreatedKey] = useState<APIKeyItem | null>(null);
  const [createMessage, setCreateMessage] = useState(t("createHint"));
  const [dialogError, setDialogError] = useState("");
  const [revealedIds, setRevealedIds] = useState<string[]>([]);
  const [creating, setCreating] = useState(false);
  const [copyFallback, setCopyFallback] = useState("");
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [models, setModels] = useState<CatalogModel[]>([]);
  const [modelsLoaded, setModelsLoaded] = useState(false);
  const [endpoint, setEndpoint] = useState("");
  const [verifying, setVerifying] = useState(false);
  const [verifyOk, setVerifyOk] = useState<boolean | null>(null);
  const [verifyMessage, setVerifyMessage] = useState("");
  const [actError, setActError] = useState("");
  const dialogSessionRef = useRef(0);
  const createdKeyRef = useRef<APIKeyItem | null>(null);
  createdKeyRef.current = createdKey;
  const message = useToast((s) => s.message);
  const setMessage = useToast((s) => s.setMessage);
  const createSchema = useMemo(
    () =>
      z.object({
        name: z.string().trim().min(1, t("nameRequired")),
        allowlist: z.array(z.string()),
        rpm: z.string().refine((value) => optionalPositiveInt(value) !== "invalid", t("limitInvalid")),
        concurrency: z.string().refine((value) => optionalPositiveInt(value) !== "invalid", t("limitInvalid")),
      }),
    [t],
  );
  const form = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    defaultValues: defaultForm,
  });

  useEffect(() => {
    if (keysCreateQueryOpen(window.location.search)) {
      handleCreateOpenChange(true);
    }
    // 进入页面由 useListResource 拉列表；?create=1 时直接打开创建弹窗。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!createOpen) {
      return;
    }
    const session = dialogSessionRef.current;
    let cancelled = false;
    setModelsLoaded(false);
    async function loadSupport() {
      const host = window.location.host;
      const [modelsRes, docsRes] = await Promise.all([
        fetch(`${apiBase}/v1/public/models?limit=100`, { credentials: "include" }),
        fetch(`${apiBase}/v1/public/docs-context?host=${encodeURIComponent(host)}`, { credentials: "include" }),
      ]);
      if (cancelled || session !== dialogSessionRef.current) {
        return;
      }
      const modelsBody = await readResponseBody(modelsRes);
      if (cancelled || session !== dialogSessionRef.current) {
        return;
      }
      if (modelsRes.ok) {
        setModels(((modelsBody as { items?: CatalogModel[] }).items || []) as CatalogModel[]);
        setModelsLoaded(true);
      } else {
        setModels([]);
        setModelsLoaded(false);
      }
      const docsBody = await readResponseBody(docsRes);
      if (cancelled || session !== dialogSessionRef.current) {
        return;
      }
      if (docsRes.ok) {
        const domain = ((docsBody as { brand?: { api_domain?: string } }).brand?.api_domain || host).replace(/\/$/, "");
        setEndpoint(`https://${domain}/v1`);
      } else {
        setEndpoint(`${window.location.origin}/v1`);
      }
    }
    void loadSupport();
    return () => {
      cancelled = true;
    };
  }, [createOpen]);

  function resetCreateDialog() {
    setCreatedKey(null);
    setCreateMessage(t("createHint"));
    setDialogError("");
    setCopyFallback("");
    setAdvancedOpen(false);
    setCreating(false);
    setVerifying(false);
    setVerifyOk(null);
    setVerifyMessage("");
    form.reset(defaultForm);
  }

  function handleCreateOpenChange(open: boolean) {
    dialogSessionRef.current += 1;
    setCreateOpen(open);
    if (!open) {
      resetCreateDialog();
    }
  }

  async function createKey(values: z.infer<typeof createSchema>) {
    const session = dialogSessionRef.current;
    const payload: { name: string; allowlist?: string[]; rpm_limit?: number; concurrency_limit?: number } = { name: values.name };
    if (values.allowlist.length > 0) {
      payload.allowlist = values.allowlist;
    }
    const rpmLimit = optionalPositiveInt(values.rpm);
    if (rpmLimit !== "empty" && rpmLimit !== "invalid") {
      payload.rpm_limit = rpmLimit;
    }
    const concLimit = optionalPositiveInt(values.concurrency);
    if (concLimit !== "empty" && concLimit !== "invalid") {
      payload.concurrency_limit = concLimit;
    }
    setCreating(true);
    setDialogError("");
    try {
      const response = await fetch(`${apiBase}/v1/me/api-keys`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      const body = await readResponseBody(response);
      if (!response.ok) {
        if (session !== dialogSessionRef.current) {
          return;
        }
        setDialogError(errorMessageFromBody(body, tc("createFailed")));
        return;
      }
      const created = ((body as { item?: APIKeyItem }).item || {}) as APIKeyItem;
      await list.reload();
      if (session !== dialogSessionRef.current) {
        return;
      }
      const listed = Array.isArray(created.allowlist) && created.allowlist.length > 0 ? created.allowlist.join(", ") : tc("unlimited");
      setCreateMessage(t("created", { id: created.id || "", name: created.name || values.name, list: listed, n: created.concurrency_limit || 5 }));
      setCreatedKey(created);
      form.reset(defaultForm);
    } catch {
      if (session !== dialogSessionRef.current) {
        return;
      }
      setDialogError(tc("listNetwork"));
    } finally {
      if (session === dialogSessionRef.current) {
        setCreating(false);
      }
    }
  }

  async function copySecret(id: string, secret?: string) {
    const value = secret ?? items.find((item) => item.id === id)?.key;
    if (!value) {
      setCopyFallback("");
      const text = t("copyMissing");
      setMessage(text);
      if (createOpen) {
        setDialogError(text);
      }
      return;
    }
    try {
      const response = await fetch(`${apiBase}/v1/me/api-keys/${id}/copy`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: "{}",
      });
      const body = await readResponseBody(response);
      if (!response.ok) {
        setCopyFallback(value);
        const text = errorMessageFromBody(body, t("actFail"));
        setMessage(text);
        if (createOpen) {
          setDialogError(text);
        }
        return;
      }
      const wrote = await copyText(value);
      if (!wrote) {
        setCopyFallback(value);
        const text = t("copyFailed");
        setMessage(text);
        if (createOpen) {
          setDialogError(text);
        }
        return;
      }
      setCopyFallback("");
      setMessage(t("copiedAudit"));
      if (createOpen) {
        setDialogError("");
        setCreateMessage(t("copiedAudit"));
      }
    } catch {
      setCopyFallback(value);
      const text = tc("listNetwork");
      setMessage(text);
      if (createOpen) {
        setDialogError(text);
      }
    }
  }

  async function verifyCreatedKey() {
    const session = dialogSessionRef.current;
    const keyID = createdKey?.id;
    const secret = createdKey?.key;
    if (!secret || !keyID) {
      setVerifyOk(false);
      setVerifyMessage(t("verifyKeyNeedSecret"));
      return;
    }
    const example = keyExampleFor(createdKey.allowlist, models, endpoint);
    if (!modelsLoaded || !models.some((item) => item.id === example.model)) {
      setVerifyOk(false);
      setVerifyMessage(t("verifyKeyNeedCatalog"));
      return;
    }
    const req = keyVerifyRequest(example.model, example.path);
    if (!req) {
      setVerifyOk(false);
      setVerifyMessage(t("verifyKeyUnsupported"));
      return;
    }
    setVerifying(true);
    try {
      const response = await fetch(`${apiBase}${req.path}`, {
        method: "POST",
        credentials: "omit",
        headers: {
          "Content-Type": "application/json",
          Authorization: `Bearer ${secret}`,
        },
        body: JSON.stringify(req.body),
      });
      const body = await readResponseBody(response);
      if (session !== dialogSessionRef.current || createdKeyRef.current?.id !== keyID) {
        return;
      }
      if (!response.ok) {
        setVerifyOk(false);
        setVerifyMessage(t("verifyKeyFail", { error: errorMessageFromBody(body, String(response.status)) }));
        return;
      }
      setVerifyOk(true);
      setVerifyMessage(t("verifyKeyOk"));
    } catch {
      if (session !== dialogSessionRef.current || createdKeyRef.current?.id !== keyID) {
        return;
      }
      setVerifyOk(false);
      setVerifyMessage(tc("listNetwork"));
    } finally {
      if (session === dialogSessionRef.current && createdKeyRef.current?.id === keyID) {
        setVerifying(false);
      }
    }
  }

  async function act(id: string, action: "rotate" | "disable" | "expire"): Promise<boolean> {
    try {
      const response = await fetch(`${apiBase}/v1/me/api-keys/${id}/${action}`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: action === "expire" ? JSON.stringify({ expires_at: new Date().toISOString() }) : "{}",
      });
      const body = await readResponseBody(response);
      if (!response.ok) {
        const text = errorMessageFromBody(body, t("actFail"));
        setActError(text);
        setMessage(text);
        return false;
      }
      setActError("");
      setMessage(t("acted", { action }));
      await list.reload();
      return true;
    } catch {
      const text = tc("listNetwork");
      setActError(text);
      setMessage(text);
      return false;
    }
  }

  function toggleReveal(id: string) {
    setRevealedIds((current) => (current.includes(id) ? current.filter((item) => item !== id) : [...current, id]));
  }

  const createdExample = createdKey ? keyExampleFor(createdKey.allowlist, models, endpoint) : null;
  const tryModel = createdExample ? models.find((item) => item.id === createdExample.model) : undefined;
  const tryHref = tryModel ? useModelHref(tryModel) : "/app/playground";

  return (
    <Card>
      <LeadActions
        lead={<p className="text-sm text-ink-secondary">{t("keysLead")}</p>}
        actions={
          <>
            <Button type="button" variant="outline" onClick={() => void list.reload()}>
              {tc("refresh")}
            </Button>
            <Button type="button" onClick={() => handleCreateOpenChange(true)}>
              {t("createKey")}
            </Button>
          </>
        }
      />
      <ListResourceView
        snapshot={list.snapshot}
        emptyTitle={t("emptyKeys")}
        emptyDetail={t("emptyKeysDetail")}
        emptyAction={
          <Button type="button" onClick={() => handleCreateOpenChange(true)}>
            {t("createKey")}
          </Button>
        }
        loadingTitle={t("keysTitle")}
        onRetry={() => void list.reload()}
        name="keys"
      >
        <KeysList
          items={items}
          revealedIds={revealedIds}
          onCopy={(id) => void copySecret(id)}
          onToggleReveal={toggleReveal}
          onRotate={(id) => act(id, "rotate")}
          onDisable={(id) => act(id, "disable")}
          onExpire={(id) => act(id, "expire")}
          actionError={actError}
          onActionErrorClear={() => setActError("")}
        />
      </ListResourceView>
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      {!createOpen && copyFallback ? <SubmitStatus error={t("copyFailed")} selectable={copyFallback} /> : null}
      <Dialog open={createOpen} onOpenChange={handleCreateOpenChange}>
        <DialogContent className="max-h-[90vh] overflow-y-auto">
          {createdKey ? (
            <>
              <DialogHeader>
                <DialogTitle>{t("createdTitle")}</DialogTitle>
                <DialogDescription>{t("createdLead")}</DialogDescription>
              </DialogHeader>
              <dl className="grid gap-3 text-sm">
                <div>
                  <dt className="text-xs text-ink-mute">{t("endpoint")}</dt>
                  <dd>
                    <code className="th-code mt-1 block overflow-x-auto whitespace-nowrap text-[13px] text-ink">{endpoint || "/v1"}</code>
                  </dd>
                </div>
                <div>
                  <dt className="text-xs text-ink-mute">{t("secret")}</dt>
                  <dd className="mt-1 flex flex-col gap-2">
                    <code data-testid="key-secret" className="th-code block overflow-x-auto whitespace-nowrap text-[13px] text-ink">
                      {createdKey.key || maskAPIKey(createdKey.prefix)}
                    </code>
                    <Button type="button" variant="outline" size="sm" onClick={() => void copySecret(createdKey.id, createdKey.key)}>
                      {tc("copy")}
                    </Button>
                  </dd>
                </div>
                <div>
                  <dt className="text-xs text-ink-mute">{t("example")}</dt>
                  <dd className="mt-1 space-y-2">
                    <pre data-testid="key-example" className="th-code overflow-x-auto whitespace-pre-wrap break-all p-3 text-[12px] text-ink">
                      {createdExample?.curl}
                    </pre>
                    <p className="text-sm text-ink-secondary">{t("exampleEnv")}</p>
                    <Button type="button" variant="outline" size="sm" asChild>
                      <Link href="/app/docs">{t("example")}</Link>
                    </Button>
                  </dd>
                </div>
                <div>
                  <dt className="text-xs text-ink-mute">{t("tryModel")}</dt>
                  <dd className="mt-1">
                    <Button type="button" size="sm" variant="outline" asChild>
                      <Link href={tryHref}>{t("verifyCta")}</Link>
                    </Button>
                  </dd>
                </div>
                <div>
                  <dt className="text-xs text-ink-mute">{t("verifyKey")}</dt>
                  <dd className="mt-1 space-y-2">
                    <Button type="button" size="sm" disabled={verifying || !createdExample?.verifiable} onClick={() => void verifyCreatedKey()}>
                      {t("verifyKeyCta")}
                    </Button>
                    {!createdExample?.verifiable ? (
                      <p data-testid="key-verify-status" data-ok="false" className="text-sm text-ink-secondary">
                        {modelsLoaded ? t("verifyKeyUnsupported") : t("verifyKeyNeedCatalog")}
                      </p>
                    ) : verifyMessage ? (
                      <p data-testid="key-verify-status" data-ok={verifyOk === true ? "true" : "false"} className="text-sm text-ink-secondary">
                        {verifyMessage}
                      </p>
                    ) : null}
                  </dd>
                </div>
              </dl>
              {!dialogError ? <p className="text-sm text-ink-secondary">{createMessage}</p> : null}
              <SubmitStatus error={dialogError} selectable={copyFallback} />
              <DialogFooter>
                <Button type="button" onClick={() => handleCreateOpenChange(false)}>
                  {t("done")}
                </Button>
              </DialogFooter>
            </>
          ) : (
            <>
              <DialogHeader>
                <DialogTitle>{t("createKey")}</DialogTitle>
                <DialogDescription>{t("createDialogLead")}</DialogDescription>
              </DialogHeader>
              <Form {...form}>
                <form
                  className="grid gap-3"
                  onSubmit={form.handleSubmit(createKey, () => {
                    setAdvancedOpen(true);
                  })}
                >
                  <TextField control={form.control} name="name" label={t("nameLabel")} placeholder={t("namePh")} />
                  <button
                    type="button"
                    className="justify-self-start text-sm text-brand underline-offset-2 hover:underline"
                    aria-expanded={advancedOpen}
                    onClick={() => setAdvancedOpen((current) => !current)}
                  >
                    {t("advanced")}
                  </button>
                  {advancedOpen ? (
                    <div className="grid gap-3 rounded-control border border-hairline bg-canvas p-3">
                      <FormField
                        control={form.control}
                        name="allowlist"
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t("modelLimit")}</FormLabel>
                            <ModelAllowlistPicker
                              value={field.value ?? []}
                              onChange={field.onChange}
                              options={models}
                              allLabel={t("allowlistAll")}
                              searchLabel={t("modelSearch")}
                              selectedLabel={t("modelLimit")}
                            />
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                      <TextField control={form.control} name="rpm" label={t("rpm")} placeholder={t("rpmPh")} />
                      <TextField control={form.control} name="concurrency" label={t("conc")} placeholder={t("concPh")} />
                    </div>
                  ) : null}
                  {!dialogError ? <p className="text-sm text-ink-secondary">{createMessage}</p> : null}
                  <SubmitStatus error={dialogError} />
                  <DialogFooter>
                    <Button type="button" variant="outline" disabled={creating} onClick={() => handleCreateOpenChange(false)}>
                      {tc("cancel")}
                    </Button>
                    <Button type="submit" disabled={creating}>
                      {creating ? tc("submitting") : tc("create")}
                    </Button>
                  </DialogFooter>
                </form>
              </Form>
            </>
          )}
        </DialogContent>
      </Dialog>
    </Card>
  );
}
