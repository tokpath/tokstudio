"use client";
import { useI18n } from "@/components/i18n";
import { useState, type FormEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { newActionKey, request } from "@/lib/api-client";
import { CommandResultSchema, InvoiceFilesSchema } from "@/lib/contracts";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AlertText, Loading, ErrorPanel, Table, When, Status, Notice,Money,statusLabels,useCommand } from "./shared";

export function InvoiceFiles({ scope, invoiceID, upload, close, embedded = false }: { scope: string; invoiceID: string; upload: boolean; close?: () => void; embedded?: boolean }) {
  const { t } = useI18n();
  const path = `${scope}/${invoiceID}/files`;
  const data = useQuery({ queryKey: ["invoice-files", path], queryFn: () => request(path, InvoiceFilesSchema) });
  const [percent, setPercent] = useState(0);
  const command = useCommand(path, CommandResultSchema, "POST", { onUploadProgress: (loaded, total) => setPercent(total > 0 ? Math.min(100, Math.round(loaded * 100 / total)) : 0) });
  const [file, setFile] = useState<File>();
  const [error, setError] = useState("");
  const [confirmID, setConfirmID] = useState("");
  const [deleteError, setDeleteError] = useState<unknown>();
  async function remove(id: string) {
    setDeleteError(undefined);
    try {
      await request(`${path}/${id}`, CommandResultSchema, { method: "DELETE", actionKey: newActionKey() });
      setConfirmID("");
      await data.refetch();
    } catch (cause) {
      setDeleteError(cause);
    }
  }
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!file) return;
    if (file.size > 10 * 1024 * 1024) { setError("文件不得超过10MiB。"); return; }
    setError("");
    const body = new FormData();
    body.append("file", file);
    await command.run(body);
  }
  const files = data.isPending ? <Loading /> : data.isError ? <ErrorPanel error={data.error} retry={() => void data.refetch()} /> : <>
    {data.data.details && <dl className="my-5 grid gap-3 text-sm sm:grid-cols-2"><div><dt className="text-ink-mute">{t("抬头")}</dt><dd>{data.data.details.title}</dd></div><div><dt className="text-ink-mute">{t("纳税识别号")}</dt><dd>{data.data.details.tax_id}</dd></div><div><dt className="text-ink-mute">{t("接收邮箱")}</dt><dd>{data.data.details.email}</dd></div><div><dt className="text-ink-mute">{t("地址")}</dt><dd>{data.data.details.address || t("未提供")}</dd></div></dl>}
    {!!data.data.orders?.length && <div className="my-5"><h4 className="mb-3 font-medium">{t("本次开票的购买订单")}</h4><Table rows={data.data.orders} getID={item => item.order_id} columns={[{label:t("购买内容"),cell:item=><><span>{t(item.kind)}{item.name && ` · ${item.name}`}</span><details className="mt-1 text-xs text-ink-mute"><summary>{t("订单编号")}</summary><span className="break-all">{item.order_id}</span></details></>},{label:t("购买时间"),cell:item=><When value={item.created_at}/>},{label:t("订单实付金额"),cell:item=><Money value={item.amount_money} unit="MONEY"/>}]} /></div>}
    {data.data.files_available !== false && <div className="my-5"><Table rows={data.data.items} getID={item => item.id} empty={t("尚未上传实际票据文件。")} columns={[{ label: t("票据文件"), cell: item => item.filename }, { label: t("状态"), cell: item => <Status value={item.state} /> }, { label: t("上传时间"), cell: item => <When value={item.created_at} /> }, { label: t("取得原文件"), cell: item => item.state === "ready" && item.download_url.startsWith(path + "/") ? <a href={item.download_url} className="text-brand-emphasis underline">{t("授权下载")}</a> : t("尚未就绪") }, ...(upload ? [{ label: t("删除"), cell: (item: { id: string }) => <Button type="button" variant="ghost" size="sm" onClick={() => confirmID === item.id ? void remove(item.id) : setConfirmID(item.id)}>{t(confirmID === item.id ? "确认删除" : "删除")}</Button> }] : [])]} />{!!deleteError && <ErrorPanel error={deleteError} compact />}</div>}
  </>;
  const uploader = upload && <form onSubmit={submit} className="space-y-4 border-t border-hairline pt-5"><Notice>{t("上传已开具的发票（PDF、PNG 或 JPEG，不超过 10MiB），再填写发票编号并登记。")}</Notice><label className="block text-sm">{t("票据文件（PDF/PNG/JPEG，不超过10MiB）")}<Input className="mt-2" type="file" accept="application/pdf,image/png,image/jpeg" required disabled={command.busy || command.unresolved} onChange={event => setFile(event.target.files?.[0])} /></label>{error && <AlertText className="text-danger">{t(error)}</AlertText>}{!!command.error && <ErrorPanel error={command.error} />}{command.result && <Notice>{t("文件已保存，状态：")}{t(statusLabels[command.result.status] || command.result.status)}{t("。")}</Notice>}<Button disabled={command.busy || !file}>{command.busy ? t("正在上传 {percent}%，请保持页面", { percent }) : command.unresolved ? t("重试原文件上传") : t("上传文件")}</Button></form>;
  if (embedded) return <section className="mb-8 border-b border-hairline pb-8"><h3 className="text-sm font-semibold">{t("申请资料与票据")}</h3>{files}{uploader}</section>;
  return <Card className="my-6"><div className="flex items-start justify-between gap-4"><div><h2 className="text-xl font-semibold">{t("申请资料与票据")}</h2></div>{close && <Button variant="outline" onClick={close}>{t("关闭")}</Button>}</div>{files}{uploader}</Card>;
}
