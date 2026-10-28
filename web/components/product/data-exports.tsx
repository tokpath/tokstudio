"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { useI18n } from "@/components/i18n";
import { request } from "@/lib/api-client";
import { CommandResultSchema, ResourcePageSchema, type Entity } from "@/lib/contracts";
import { useProduct } from "./context";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Empty, ErrorPanel, Heading, Loading, Table, When, useCommand } from "./shared";
import { monthRange, rangeDays, ReportPeriod, type ReportRange } from "./report-period";

export const exportKinds: Record<string, string> = { financial: "经营报表", orders: "客户订单", payments: "收款记录", requests: "调用明细", procurement: "采购与供货", ledger: "账务明细" };
const states: Record<string, string> = { pending: "排队中", processing: "生成中", ready: "可下载", failed: "生成失败", expired: "已过期" };
export function DataExportsScreen({ operatorID }: { operatorID: string }) {
  const { t } = useI18n();
  const { session } = useProduct();
  const workspace = session?.kind === "staff" ? session.workspaces.find(w => w.operator.id === operatorID) : undefined;
  const has = (permission: string) => !!workspace && (workspace.permissions.includes("*") || workspace.permissions.includes(permission));
  const [kind, setKind] = useState("orders"), [range, setRange] = useState<ReportRange>(monthRange);
  const path = `/api/v1/operators/${encodeURIComponent(operatorID)}/exports`;
  const command = useCommand(path, CommandResultSchema);
  const data = useQuery({ queryKey: ["data-exports", operatorID], queryFn: ({ signal }) => request(path, ResourcePageSchema, { signal }), enabled: has("reports.export"), refetchInterval: query => query.state.data?.items.some(row => ["pending", "processing"].includes(row.status || "")) ? 3000 : false });
  useEffect(() => {
    const params = new URLSearchParams(window.location.search), k = params.get("kind"), from = params.get("from"), to = params.get("to");
    if (k && exportKinds[k]) setKind(k);
    if (from && to && Number.isFinite(Date.parse(from)) && Number.isFinite(Date.parse(to))) setRange({ from, to });
  }, [operatorID]);
  if (!has("reports.export")) return <Empty title={t("没有导出权限")}>{t("需要当前经营方的数据导出权限。")}</Empty>;
  const available = Object.entries(exportKinds).filter(([key]) => key !== "financial" || has("reports.view"));
  const selected = available.some(([key]) => key === kind) ? kind : "orders", blocked = command.busy || command.unresolved;
  const create = async (next: ReportRange) => { if (command.busy) return; setRange(next); await command.run({ kind: selected, ...next }); };
  const retry = async (row: Entity) => {
    command.reset();
    const next = { from: String(row.values.from_at), to: String(row.values.to_at) };
    setKind(String(row.values.kind)); setRange(next); await command.run({ kind: String(row.values.kind), ...next });
  };
  const period = (row: Entity) => { const days = rangeDays({ from: String(row.values.from_at), to: String(row.values.to_at) }); return `${days.from} — ${days.to}`; };
  const status = (row: Entity) => <div><span className="rounded-full bg-canvas px-2 py-1 text-xs">{t(states[row.status || ""] || "待处理")}</span>{row.status === "failed" && <p className="mt-2 max-w-60 text-xs text-danger">{t(row.values.error_code === "range_too_large" ? "数据较多，请缩小日期范围后重新生成。" : row.values.error_code === "format_changed" ? "导出格式已更新，请重新生成。" : "生成失败，请重新生成或稍后再试。")}</p>}</div>;
  const action = (row: Entity) => row.status === "ready" && (row.values.kind !== "financial" || has("reports.view")) && typeof row.values.download_url === "string"
    ? <div><Button asChild size="sm" variant="outline"><a href={row.values.download_url} download>{t("下载 CSV")}</a></Button><p className="mt-2 text-xs text-ink-mute">{t("下载有效期至")} <When value={String(row.values.expires_at)} /></p></div>
    : ["failed", "expired"].includes(row.status || "") ? <Button variant="ghost" size="sm" disabled={blocked || (row.values.kind === "financial" && !has("reports.view"))} onClick={() => retry(row)}>{t("重新生成")}</Button>
      : <span className="text-sm text-ink-mute">{t(row.status === "ready" ? "没有查看权限" : "生成中")}</span>;
  const rows = data.data?.items || [];
  return <>
    <Heading title={t("数据导出")} description={t("选择内容和日期，生成可用表格软件打开的 CSV。仅包含当前经营方有权查看的数据。")} />
    <Card className="mb-6">
      <label className="mb-4 block max-w-sm text-sm">{t("导出内容")}<select value={selected} disabled={blocked} onChange={e => { command.reset(); setKind(e.target.value); }} className="mt-2 min-h-11 w-full rounded-control border border-hairline bg-canvas-raised px-3">{available.map(([value, label]) => <option key={value} value={value}>{t(label)}</option>)}</select></label>
      <ReportPeriod range={range} onApply={create} label={command.busy ? "正在提交" : "生成 CSV"} disabled={blocked} autoApply={false} preserveRange />
      {!!command.error && <div className="mt-4"><ErrorPanel error={command.error} retry={() => void command.run({})} />{command.unresolved && <p className="mt-2 text-sm text-ink-mute">{t("提交结果待确认，请重试原操作，避免重复生成。")}</p>}</div>}
      {command.result && <p role="status" className="mt-4 text-sm text-brand-emphasis">{t("导出已提交，生成后可在下方下载。")}</p>}
    </Card>
    <h2 className="mb-4 text-lg font-semibold">{t("导出记录")}</h2>
    {data.isPending ? <Loading /> : data.isError ? <ErrorPanel error={data.error} retry={() => void data.refetch()} /> : !rows.length ? <Empty>{t("暂无导出记录，选择内容和日期即可生成。")}</Empty> : <>
      <div className="space-y-4 md:hidden">{rows.map(row => <Card key={row.id}>
        <div className="mb-3 flex items-start justify-between gap-3"><p className="font-medium">{t(exportKinds[String(row.values.kind)] || "数据导出")}</p>{status(row)}</div>
        <p className="mb-2 text-sm">{period(row)}</p><p className="mb-4 text-xs text-ink-mute"><When value={String(row.values.created_at)} />{row.status === "ready" && <> · {t("记录数")} {String(row.values.row_count)}</>}</p>{action(row)}
      </Card>)}</div>
      <div className="hidden md:block"><Table rows={rows} getID={row => row.id} columns={[
        { label: t("导出内容"), cell: row => <div><p className="font-medium">{t(exportKinds[String(row.values.kind)] || "数据导出")}</p><p className="mt-1 text-xs text-ink-mute"><When value={String(row.values.created_at)} /></p></div> },
        { label: t("日期范围"), cell: period }, { label: t("记录数"), cell: row => row.status === "ready" ? String(row.values.row_count) : "—" },
        { label: t("状态"), cell: status }, { label: t("下载"), cell: action },
      ]} /></div>
    </>}
    {has("reports.view") && <div className="mt-6"><Link href={`/manage/${encodeURIComponent(operatorID)}/reports`} className="text-sm text-brand-emphasis">{t("查看经营报表")}</Link></div>}
  </>;
}
