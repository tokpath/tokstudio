"use client";
import { useI18n } from "@/components/i18n";
import { useState, type FormEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { z } from "zod";
import { request } from "@/lib/api-client";
import { CommandResultSchema } from "@/lib/contracts";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { AlertText, Loading, ErrorPanel, Table, When, Status, Notice,statusLabels,useCommand } from "./shared";
const FilesSchema=z.object({items:z.array(z.object({id:z.string(),filename:z.string(),content_type:z.string(),size_bytes:z.number().int(),state:z.string(),created_at:z.string(),download_url:z.string()})),details:z.object({title:z.string(),tax_id:z.string(),email:z.string(),address:z.string().optional()}).optional()});
export function InvoiceFiles({scope,invoiceID,upload,close}:{scope:string;invoiceID:string;upload:boolean;close:()=>void}){
  const { t } = useI18n();
 const path=`${scope}/${invoiceID}/files`;const data=useQuery({queryKey:["invoice-files",path],queryFn:()=>request(path,FilesSchema)});const command=useCommand(path,CommandResultSchema);const [file,setFile]=useState<File>(),[error,setError]=useState("");
 async function submit(event:FormEvent){event.preventDefault();if(!file)return;if(file.size>10*1024*1024){setError("文件不得超过10MiB。");return}setError("");const body=new FormData();body.append("file",file);await command.run(body)}
 return <Card className="my-6"><div className="flex items-start justify-between gap-4"><div><h2 className="text-xl font-semibold">{t("真实票据与收件资料")}</h2><p className="mt-2 break-all font-mono text-xs">{invoiceID}</p></div><Button variant="outline" onClick={close}>{t("关闭")}</Button></div>{data.isPending?<Loading/>:data.isError?<ErrorPanel error={data.error} retry={()=>void data.refetch()}/>:<>{data.data.details&&<dl className="my-5 grid gap-3 text-sm sm:grid-cols-2"><div><dt className="text-ink-mute">{t("抬头")}</dt><dd>{data.data.details.title}</dd></div><div><dt className="text-ink-mute">{t("纳税识别号")}</dt><dd>{data.data.details.tax_id}</dd></div><div><dt className="text-ink-mute">{t("接收邮箱")}</dt><dd>{data.data.details.email}</dd></div><div><dt className="text-ink-mute">{t("地址")}</dt><dd>{data.data.details.address||t("未提供")}</dd></div></dl>}<div className="my-5"><Table rows={data.data.items} getID={f=>f.id} empty={t("尚未上传实际票据文件。")} columns={[{label:t("票据文件"),cell:f=>f.filename},{label:t("状态"),cell:f=><Status value={f.state}/>},{label:t("上传时间"),cell:f=><When value={f.created_at}/>},{label:t("取得原文件"),cell:f=>f.state==="ready"&&f.download_url.startsWith(path+"/")?<a href={f.download_url} className="text-brand-emphasis underline">{t("授权下载")}</a>:t("尚未就绪")}]}/></div></>}
 {upload&&<form onSubmit={submit} className="space-y-4 border-t border-hairline pt-5"><Notice>{t("上传已出具的真实票据文件。S3对象保持私有，下载时重新核对经营方或申请人身份。上传不代表已经开票，还需登记真实凭证编号。")}</Notice><label className="block text-sm">{t("票据文件（PDF/PNG/JPEG，不超过10MiB）")}<Input className="mt-2" type="file" accept="application/pdf,image/png,image/jpeg" required disabled={command.busy||command.unresolved} onChange={event=>setFile(event.target.files?.[0])}/></label>{error&&<AlertText className="text-danger">{t(error)}</AlertText>}{!!command.error&&<ErrorPanel error={command.error}/>}<Button disabled={command.busy||!file}>{command.unresolved?t("重试原文件上传"):t("上传真实私有文件")}</Button>{command.result&&<Notice>{t("文件已保存，状态：")}{t(statusLabels[command.result.status] || command.result.status)}{t("。")}</Notice>}</form>}
 </Card>
}
