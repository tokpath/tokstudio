"use client";

import { useEffect, useState } from "react";
import { z } from "zod";
import { useI18n } from "@/components/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { ErrorPanel, useCommand } from "./shared";

const Evidence = z.object({ id: z.string().uuid(), status: z.literal("ready") });
const marker = "tokenhub-evidence:";

export function FinanceEvidenceUpload({ operatorID, disabled, attach, onPendingChange }: { operatorID: string; disabled: boolean; attach: (reference: string) => void; onPendingChange: (pending: boolean) => void }) {
  const { t } = useI18n();
  const [file, setFile] = useState<File>();
  const command = useCommand(`/api/v1/operators/${encodeURIComponent(operatorID)}/evidence`, Evidence);
  useEffect(() => { onPendingChange(command.busy || command.unresolved); return () => onPendingChange(false); }, [command.busy, command.unresolved, onPendingChange]);
  return <details className="rounded-control border border-hairline p-4">
    <summary className="cursor-pointer text-sm text-ink-secondary">{t("附加付款凭据（可选）")}</summary>
    <div className="mt-4 space-y-3">
      <p className="text-xs leading-6 text-ink-mute">{t("上传 PDF、PNG 或 JPEG，最多 10 MiB。上传成功后自动关联本次核实依据；仍须核实真实到账。")}</p>
      <Input type="file" aria-label={t("选择付款凭据")} accept="application/pdf,image/png,image/jpeg" disabled={disabled || command.busy || command.unresolved} onChange={event => { setFile(event.target.files?.[0]); command.reset(); }} />
      <Button type="button" size="sm" variant="outline" disabled={disabled || command.busy || (!file && !command.unresolved) || !!command.result} onClick={async () => { const body = new FormData(); if(file) body.set("file",file); const result = await command.run(body); if(result) attach(marker+result.id); }}>{command.unresolved ? t("重试原附件上传") : t("上传并关联凭据")}</Button>
      {command.unresolved && <Button type="button" variant="ghost" size="sm" onClick={() => { command.reset(); setFile(undefined); }}>{t("放弃本次附件")}</Button>}
      {command.error ? <ErrorPanel error={command.error} /> : null}
      {command.result && <p role="status" className="text-sm text-success">{t("凭据已关联")}{file ? ` · ${file.name}` : ""}</p>}
    </div>
  </details>;
}

export function FinanceEvidenceLinks({ value, operatorID }: { value: unknown; operatorID?: string }) {
  const { t } = useI18n();
  const ids = [...new Set(JSON.stringify(value).match(/tokenhub-evidence:[0-9a-f-]{36}/gi) || [])].map(item => item.slice(marker.length));
  if (!operatorID || !ids.length) return null;
  return <div className="my-4 flex flex-wrap gap-3">{ids.map((id,index) => <a key={id} className="text-sm text-brand-emphasis underline" href={`/api/v1/operators/${encodeURIComponent(operatorID)}/evidence/${encodeURIComponent(id)}/content`}>{t("查看付款凭据 {number}", {number:index+1})}</a>)}</div>;
}
