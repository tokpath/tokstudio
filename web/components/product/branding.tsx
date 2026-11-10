"use client";
import { useEffect, useState, type ReactNode } from "react";
import { z } from "zod";
import { request, newActionKey } from "@/lib/api-client";
import { CommandResultSchema, type Entity, type Operation } from "@/lib/contracts";
import { useNavigationBlock } from "@/lib/navigation-guard";
import { useI18n } from "@/components/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { When } from "./shared";
import { Card } from "@/components/ui/card";
import { ErrorPanel, Notice, useCommand } from "./shared";

type Theme = {primary_color?:string; background_color?:string; text_color?:string; logo_url?:string; support_url?:string};
function previewURL(url: string|undefined, operator: string) {
 if (!url) return undefined;
 return url.startsWith("/v1/branding/assets/") ? `/api/v1/operators/${encodeURIComponent(operator)}/branding/assets/${url.split("/")[4]}/content` : url;
}
export function AppearanceCard({row,children}:{row:Entity;children:ReactNode}) {
 const {t}=useI18n(), theme=row.values.effective_theme as Theme || {};
 return <Card><p className="mb-4 font-semibold">{String(row.values.name || "")}</p><div className="mb-5 flex items-center gap-4 rounded-control border border-hairline p-6" style={{backgroundColor:theme.background_color,color:theme.text_color}}>
 {/* eslint-disable-next-line @next/next/no-img-element */}
 {theme.logo_url && <img src={previewURL(theme.logo_url,row.id)} alt="Logo" className="h-14 max-w-40 object-contain" />}
 <div><p className="font-semibold">{t("网站外观预览")}</p><span className="mt-2 inline-block rounded-control px-4 py-2 text-sm text-white" style={{backgroundColor:theme.primary_color}}>{t("主色")}</span></div></div>{children}</Card>;
}
export function AppearanceForm({operation,row,operatorID,onBlockedChange}:{operation:Operation;row:Entity;operatorID:string;onBlockedChange:(v:boolean)=>void}) {
 const {t}=useI18n(), original=row.values as Theme, effective=row.values.effective_theme as Theme || {};
 const [values,setValues]=useState<Theme>({...original});
 const defaults=row.values.default_theme as Theme || effective;
 const [colorsReset,setColorsReset]=useState(false);
 const [logoReset,setLogoReset]=useState(false);
 const [uploading,setUploading]=useState(false),[progress,setProgress]=useState(0),[error,setError]=useState<unknown>(),[local,setLocal]=useState("");
 const command=useCommand(operation.path,CommandResultSchema);
 const blocked=uploading||command.busy||command.unresolved, locked=blocked||!!command.result;
 useNavigationBlock(blocked);
 useEffect(()=>{onBlockedChange(blocked);return()=>onBlockedChange(false)},[blocked,onBlockedChange]);
 useEffect(()=>()=>{if(local)URL.revokeObjectURL(local)},[local]);
 async function upload(file?:File){
  if(!file)return;setError(undefined);
  if(file.size>2*1024*1024||!["image/png","image/jpeg","image/webp"].includes(file.type)){setError(new Error(t("请选择不超过 2 MB 的 PNG、JPEG 或 WebP 图片。")));return}
  setUploading(true);setProgress(0);
  try{const body=new FormData();body.append("file",file);const result=await request(`/api/v1/operators/${encodeURIComponent(operatorID)}/branding/logo`,z.object({asset:z.object({url:z.string()})}),{method:"POST",body,actionKey:newActionKey(),onUploadProgress:(done,total)=>setProgress(total?Math.min(100,Math.round(done/total*100)):0)});setValues(v=>({...v,logo_url:result.asset.url}));setLocal(URL.createObjectURL(file));setLogoReset(false);}catch(e){setError(e)}finally{setUploading(false)}
 }
 async function save(e:React.FormEvent){e.preventDefault();if(locked&&!command.unresolved)return;const clean:Record<string,string|null>={};for(const name of ["primary_color","background_color","text_color","support_url"] as const)clean[name]=values[name]?.trim()||null;clean.logo_url=values.logo_url??null;await command.run({expected_version:row.version||0,values:clean})}
 return <form onSubmit={save} className="space-y-6"><fieldset disabled={locked} className="space-y-5">
 <div><label className="block text-sm">{t("网站 Logo")}<input aria-label={t("网站 Logo")} type="file" accept="image/png,image/jpeg,image/webp" className="mt-3 block w-full text-sm" onChange={e=>{void upload(e.target.files?.[0]);e.target.value=""}} /></label><p className="mt-2 text-xs text-ink-mute">{t("请选择不超过 2 MB 的 PNG、JPEG 或 WebP 图片。")}</p>
 {(local||(logoReset?defaults.logo_url:effective.logo_url))&& <div className="my-3">{/* eslint-disable-next-line @next/next/no-img-element */}<img src={local||previewURL(logoReset?defaults.logo_url:effective.logo_url,operatorID)} alt="Logo" className="h-16 max-w-48 object-contain" /></div>}
 {uploading&&<p role="status" className="mt-3 text-sm">{progress===100?t("正在保存图片…"):t("正在上传…")+` ${progress}%`}</p>}
 {values.logo_url!==undefined&&<Button type="button" size="sm" variant="ghost" onClick={()=>{setValues(v=>({...v,logo_url:undefined}));setLocal("");setLogoReset(true)}}>{t("使用默认 Logo")}</Button>}</div>
 {([["primary_color","主色"],["background_color","背景色"],["text_color","文字色"]] as const).map(([key,label])=><label className="block text-sm" key={key}>{t(label)}<Input className="mt-2" type="color" value={values[key]||(colorsReset?defaults[key]:effective[key])||"#ffffff"} onInput={e=>{const value=e.currentTarget.value;setValues(v=>({...v,[key]:value}))}} onChange={e=>{const value=e.currentTarget.value;setValues(v=>({...v,[key]:value}))}}/></label>)}
 <Button type="button" size="sm" variant="ghost" onClick={()=>{setValues(v=>({...v,primary_color:undefined,background_color:undefined,text_color:undefined}));setColorsReset(true)}}>{t("使用默认配色")}</Button>
 <label className="block text-sm">{t("客户支持网址（可选）")}<Input type="url" className="mt-2" value={values.support_url||""} placeholder="https://" onChange={e=>{const value=e.currentTarget.value;setValues(v=>({...v,support_url:value}))}}/></label>
 <p className="text-xs leading-6 text-ink-mute">{t("未单独设置的项目使用默认值。上传图片后，保存外观才会在网站生效。")}</p></fieldset>
 {!!error&&<ErrorPanel error={error}/>}{!!command.error&&<ErrorPanel error={command.error}/>}
 {command.unresolved&&<Notice>{t("请保持此办理面板，查询业务记录或重试本次操作。重试会复用原提交内容与同一操作编号，避免重复办理。")}</Notice>}
 {command.result?<p role="status" className="text-success">{t("网站外观已保存。")}</p>:<Button type="submit" disabled={uploading||command.busy}>{t(command.busy?"正在提交…":command.unresolved?"重试本次操作":"保存外观")}</Button>}</form>
}
export function DomainCard({row,children,verify}:{row:Entity;children:ReactNode;verify?:Operation}){
 const {t}=useI18n(),v=row.values,records=(v.dns_records||[]) as {type:string;name:string;value:string}[];
 const check=useCommand(verify?.path.replace("{id}",encodeURIComponent(row.id))||"",CommandResultSchema);
 return <Card><div className="flex flex-wrap justify-between gap-3"><div><p className="font-semibold break-all">{String(v.hostname)}</p><p className="mt-1 text-sm text-ink-secondary">{String(v.operator_name)}</p></div><p className="text-sm font-medium">{t(String(v.state_label))}</p></div>
 {row.status!=="active"&&records.length>0&&<div className="my-5 space-y-3"><p className="text-sm">{t("在域名服务商添加以下 DNS 记录。记录生效可能需要等待，系统会自动复查。")}</p>{records.map((r,i)=><div key={i} className="rounded-control bg-canvas p-3 text-sm"><p className="font-medium">{r.type}</p><div className="mt-2 break-all"><span className="text-ink-mute">{t("记录名称")} · </span>{r.name}</div><div className="mt-2 break-all"><span className="text-ink-mute">{t("记录值")} · </span>{r.value}</div><Button type="button" size="sm" variant="ghost" onClick={()=>navigator.clipboard.writeText(r.value)}>{t("复制记录值")}</Button></div>)}</div>}
 {v.routing_note&&row.status!=="active"&&<p className="my-3 text-sm text-ink-secondary">{t(String(v.routing_note))}</p>}
 {!!v.failure_reason&&<p className="my-3 text-sm text-danger">{t(String(v.failure_reason))}</p>}
 {!!v.api_base_url&&<p className="my-3 break-all text-sm">{t("API入口")} · {String(v.api_base_url)}</p>}
 {!!v.checked_at&&<p className="my-3 text-xs text-ink-mute">{t("检查时间")} · <When value={String(v.checked_at)} /></p>}
 {!!check.error&&<ErrorPanel error={check.error}/>}
 <div className="flex flex-wrap items-center gap-2">{verify&&row.allowed_actions?.includes("verify")&&<Button type="button" size="sm" variant="outline" disabled={check.busy} onClick={()=>check.run({version:row.version})}>{t(check.busy?"正在检查…":"重新检查")}</Button>}{children}</div></Card>
}
