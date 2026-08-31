"use client";

import { useEffect, useMemo, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import { useTranslations } from "next-intl";
import { z } from "zod";
import { ConfirmButton } from "@/components/confirm-button";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { TextField } from "@/components/text-field";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Form } from "@/components/ui/form";
import { apiBase } from "@/lib/api";
import { useToast } from "@/lib/toast";

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

export function parseAllowlist(raw: string): string[] {
  return raw
    .split(/[,，]/)
    .map((item) => item.trim())
    .filter(Boolean);
}

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

type KeysListProps = {
  items: APIKeyItem[];
  revealedIds?: string[];
  onCopy?: (id: string) => void;
  onToggleReveal?: (id: string) => void;
  onRotate?: (id: string) => void;
  onDisable?: (id: string) => void;
  onExpire?: (id: string) => void;
};

export function KeysList({
  items,
  revealedIds = [],
  onCopy,
  onToggleReveal,
  onRotate,
  onDisable,
  onExpire,
}: KeysListProps) {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  if (items.length === 0) {
    return <EmptyLedger title={t("emptyKeys")} detail={t("emptyKeysDetail")} />;
  }
  const revealed = new Set(revealedIds);
  return (
    <div className="overflow-x-auto rounded-card border border-hairline">
      <table className="w-full text-left text-sm">
        <thead className="bg-canvas-raised text-ink-mute">
          <tr>
            <th className="th-eyebrow px-4 py-3 font-medium">{t("colName")}</th>
            <th className="th-eyebrow px-4 py-3 font-medium">{t("colKey")}</th>
            <th className="th-eyebrow px-4 py-3 font-medium">{t("colStatus")}</th>
            <th className="th-eyebrow px-4 py-3 font-medium">{t("colLimits")}</th>
            <th className="th-eyebrow px-4 py-3 font-medium">{t("allowTitle")}</th>
            <th className="th-eyebrow px-4 py-3 font-medium">{t("colLastUsed")}</th>
            <th className="th-eyebrow px-4 py-3 font-medium">{t("colActions")}</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item) => {
            const isRevealed = revealed.has(item.id);
            return (
              <tr key={item.id} className="border-t border-hairline hover:bg-brand-soft/40">
                <td className="px-4 py-3 text-ink">{item.name}</td>
                <td className="px-4 py-3">
                  <div className="flex flex-wrap items-center gap-2">
                    <code className="th-code break-all text-[13px] text-ink">
                      {maskAPIKey(item.prefix, item.key, isRevealed)}
                    </code>
                    {onCopy ? (
                      <Button size="sm" variant="outline" onClick={() => onCopy(item.id)}>
                        {tc("copy")}
                      </Button>
                    ) : null}
                    {onToggleReveal ? (
                      <Button size="sm" variant="ghost" onClick={() => onToggleReveal(item.id)}>
                        {isRevealed ? t("hide") : t("reveal")}
                      </Button>
                    ) : null}
                  </div>
                </td>
                <td className="px-4 py-3">
                  <Badge tone={statusTone(item)}>{item.status}</Badge>
                </td>
                <td className="px-4 py-3 text-ink">
                  {item.rpm_limit ? `RPM ${item.rpm_limit}` : ""}
                  {item.concurrency_limit ? t("concurrency", { n: item.concurrency_limit }) : ""}
                </td>
                <td className="px-4 py-3 text-ink-secondary">
                  {t("allowlistLine", { list: item.allowlist?.length ? item.allowlist.join(", ") : tc("unlimited") })}
                </td>
                <td className="px-4 py-3 text-ink-mute">{formatWhen(item.last_used_at, t("neverUsed"))}</td>
                <td className="px-4 py-3">
                  <div className="flex flex-wrap gap-2">
                    {onRotate ? (
                      <ConfirmButton
                        size="sm"
                        variant="outline"
                        title={t("rotateConfirmTitle")}
                        description={t("rotateConfirm")}
                        onConfirm={() => onRotate(item.id)}
                      >
                        {t("rotate")}
                      </ConfirmButton>
                    ) : null}
                    {onDisable ? (
                      <ConfirmButton
                        size="sm"
                        variant="outline"
                        title={t("disableConfirmTitle")}
                        description={t("disableConfirm")}
                        onConfirm={() => onDisable(item.id)}
                      >
                        {t("disable")}
                      </ConfirmButton>
                    ) : null}
                    {onExpire ? (
                      <ConfirmButton
                        size="sm"
                        variant="outline"
                        title={t("expireConfirmTitle")}
                        description={t("expireConfirm")}
                        onConfirm={() => onExpire(item.id)}
                      >
                        {t("expire")}
                      </ConfirmButton>
                    ) : null}
                  </div>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

const defaultForm = { name: "default", allowlist: "", rpm: "", concurrency: "" };

export default function KeysPanel() {
  const t = useTranslations("user");
  const tc = useTranslations("common");
  const [items, setItems] = useState<APIKeyItem[]>([]);
  const [createOpen, setCreateOpen] = useState(false);
  const [createdKey, setCreatedKey] = useState<APIKeyItem | null>(null);
  const [createMessage, setCreateMessage] = useState(t("createHint"));
  const [revealedIds, setRevealedIds] = useState<string[]>([]);
  const [creating, setCreating] = useState(false);
  const message = useToast((s) => s.message);
  const setMessage = useToast((s) => s.setMessage);
  const createSchema = useMemo(
    () =>
      z.object({
        name: z.string().trim().min(1, t("nameRequired")),
        allowlist: z.string(),
        rpm: z.string(),
        concurrency: z.string(),
      }),
    [t],
  );
  const form = useForm<z.infer<typeof createSchema>>({
    resolver: zodResolver(createSchema),
    defaultValues: defaultForm,
  });

  async function refresh(silent = false) {
    const response = await fetch(`${apiBase}/v1/me/api-keys`, { credentials: "include" });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || tc("notLoggedIn"));
      return;
    }
    setItems(body.items || []);
    if (!silent) {
      setMessage(t("refreshed"));
    }
  }

  useEffect(() => {
    void refresh(true);
    // 进入页面拉一次列表；文案来自当前 locale。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function resetCreateDialog() {
    setCreatedKey(null);
    setCreateMessage(t("createHint"));
    form.reset(defaultForm);
  }

  function handleCreateOpenChange(open: boolean) {
    setCreateOpen(open);
    if (!open) {
      resetCreateDialog();
    }
  }

  async function createKey(values: z.infer<typeof createSchema>) {
    const models = parseAllowlist(values.allowlist);
    const payload: { name: string; allowlist?: string[]; rpm_limit?: number; concurrency_limit?: number } = { name: values.name };
    if (models.length > 0) {
      payload.allowlist = models;
    }
    const rpmLimit = Number(values.rpm);
    if (values.rpm && Number.isFinite(rpmLimit) && rpmLimit > 0) {
      payload.rpm_limit = rpmLimit;
    }
    const concLimit = Number(values.concurrency);
    if (values.concurrency && Number.isFinite(concLimit) && concLimit > 0) {
      payload.concurrency_limit = concLimit;
    }
    setCreating(true);
    try {
      const response = await fetch(`${apiBase}/v1/me/api-keys`, {
        method: "POST",
        credentials: "include",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
      });
      const body = await response.json();
      if (!response.ok) {
        setCreateMessage(body.error?.message || tc("createFailed"));
        return;
      }
      const created = (body.item || {}) as APIKeyItem;
      const listed = Array.isArray(created.allowlist) && created.allowlist.length > 0 ? created.allowlist.join(", ") : tc("unlimited");
      setCreateMessage(t("created", { id: created.id || "", name: created.name || values.name, list: listed, n: created.concurrency_limit || 5 }));
      setCreatedKey(created);
      form.reset(defaultForm);
      await refresh(true);
    } finally {
      setCreating(false);
    }
  }

  async function writeClipboard(secret: string) {
    if (typeof navigator !== "undefined" && navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(secret);
    }
  }

  async function copySecret(id: string, secret?: string) {
    const value = secret ?? items.find((item) => item.id === id)?.key;
    if (!value) {
      setMessage(t("copyMissing"));
      return;
    }
    const response = await fetch(`${apiBase}/v1/me/api-keys/${id}/copy`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: "{}",
    });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || t("actFail"));
      return;
    }
    await writeClipboard(value);
    setMessage(t("copiedAudit"));
  }

  async function act(id: string, action: "rotate" | "disable" | "expire") {
    const response = await fetch(`${apiBase}/v1/me/api-keys/${id}/${action}`, {
      method: "POST",
      credentials: "include",
      headers: { "Content-Type": "application/json" },
      body: action === "expire" ? JSON.stringify({ expires_at: new Date().toISOString() }) : "{}",
    });
    const body = await response.json();
    if (!response.ok) {
      setMessage(body.error?.message || t("actFail"));
      return;
    }
    setMessage(t("acted", { action }));
    await refresh(true);
  }

  function toggleReveal(id: string) {
    setRevealedIds((current) => (current.includes(id) ? current.filter((item) => item !== id) : [...current, id]));
  }

  return (
    <Card>
      <div className="mb-4 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <CardTitle className="mb-2">{t("keysTitle")}</CardTitle>
          <p className="text-sm text-ink-secondary">{t("keysLead")}</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" onClick={() => void refresh()}>
            {tc("refresh")}
          </Button>
          <Button type="button" onClick={() => setCreateOpen(true)}>
            {t("createKey")}
          </Button>
        </div>
      </div>
      <KeysList
        items={items}
        revealedIds={revealedIds}
        onCopy={(id) => void copySecret(id)}
        onToggleReveal={toggleReveal}
        onRotate={(id) => void act(id, "rotate")}
        onDisable={(id) => void act(id, "disable")}
        onExpire={(id) => void act(id, "expire")}
      />
      <p className="mt-3 text-sm text-ink-secondary">{message}</p>
      <Dialog open={createOpen} onOpenChange={handleCreateOpenChange}>
        <DialogContent className="max-h-[90vh] overflow-y-auto">
          {createdKey ? (
            <>
              <DialogHeader>
                <DialogTitle>{t("createdTitle")}</DialogTitle>
                <DialogDescription>{t("createdLead")}</DialogDescription>
              </DialogHeader>
              <div className="flex flex-col gap-2 rounded-control border border-hairline bg-canvas px-3 py-3">
                <code className="th-code break-all text-sm text-ink">{createdKey.key || maskAPIKey(createdKey.prefix)}</code>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => void copySecret(createdKey.id, createdKey.key)}
                >
                  {tc("copy")}
                </Button>
              </div>
              <p className="text-sm text-ink-secondary">{createMessage}</p>
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
                <form className="grid gap-3" onSubmit={form.handleSubmit(createKey)}>
                  <TextField control={form.control} name="name" label={t("nameLabel")} placeholder={t("namePh")} />
                  <h3 className="text-lg font-medium">{t("allowTitle")}</h3>
                  <TextField control={form.control} name="allowlist" label={t("allowTitle")} placeholder={t("allowPh")} />
                  <TextField control={form.control} name="rpm" label={t("rpm")} placeholder={t("rpmPh")} />
                  <TextField control={form.control} name="concurrency" label={t("conc")} placeholder={t("concPh")} />
                  <p className="text-sm text-ink-secondary">{createMessage}</p>
                  <DialogFooter>
                    <Button type="button" variant="outline" disabled={creating} onClick={() => handleCreateOpenChange(false)}>
                      {tc("cancel")}
                    </Button>
                    <Button type="submit" disabled={creating}>
                      {tc("create")}
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
