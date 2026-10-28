"use client";
import { useEffect, useState, type FormEvent } from "react";
import { useI18n } from "@/components/i18n";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

export type ReportRange = { from: string; to: string };
export function localDay(date: Date) { return `${date.getFullYear()}-${String(date.getMonth()+1).padStart(2,"0")}-${String(date.getDate()).padStart(2,"0")}`; }
export function dateRange(from: string, to: string, now = new Date()): ReportRange {
  const parse = (value: string) => { const [y,m,d] = value.split("-").map(Number); const date=new Date(y,m-1,d); if (!/^\d{4}-\d{2}-\d{2}$/.test(value) || localDay(date)!==value) throw new Error("请选择有效日期范围。"); return date; };
  const start=parse(from), last=parse(to), end=new Date(last.getFullYear(),last.getMonth(),last.getDate()+1);
  if (start>last || start>=now || end.getTime()-start.getTime()>366*86400000) throw new Error("请选择有效日期范围，最多一年，开始日期不能晚于今天。");
  return {from:start.toISOString(),to:new Date(Math.min(end.getTime(),now.getTime())).toISOString()};
}
export function monthRange() { const now=new Date(); return dateRange(localDay(new Date(now.getFullYear(),now.getMonth(),1)),localDay(now),now); }
export function rangeDays(range: ReportRange) { return {from:localDay(new Date(range.from)),to:localDay(new Date(new Date(range.to).getTime()-1))}; }

export function ReportPeriod({range,onApply,label="查询",disabled=false,autoApply=true,preserveRange=false}:{range:ReportRange;onApply:(range:ReportRange)=>unknown;label?:string;disabled?:boolean;autoApply?:boolean;preserveRange?:boolean}) {
  const {t}=useI18n(); const [days,setDays]=useState(()=>rangeDays(range)); const [error,setError]=useState("");
  useEffect(()=>{setDays(rangeDays(range));setError("");},[range.from,range.to]);
  function submit(event:FormEvent){event.preventDefault();try{const original=rangeDays(range),next=preserveRange&&original.from===days.from&&original.to===days.to?range:dateRange(days.from,days.to);setError("");void onApply(next);}catch(error){setError((error as Error).message);}}
  function preset(kind:string){const now=new Date();let from=new Date(now.getFullYear(),now.getMonth(),1),to=now;if(kind==="today")from=new Date(now.getFullYear(),now.getMonth(),now.getDate());if(kind==="week")from=new Date(now.getFullYear(),now.getMonth(),now.getDate()-6);if(kind==="last"){from=new Date(now.getFullYear(),now.getMonth()-1,1);to=new Date(now.getFullYear(),now.getMonth(),0);}const next={from:localDay(from),to:localDay(to)};setDays(next);setError("");if(autoApply)void onApply(dateRange(next.from,next.to));}
  return <form onSubmit={submit} className="space-y-4"><div className="flex flex-wrap gap-2" aria-label={t("常用日期范围")}>{[["today","今天"],["week","近7天"],["month","本月"],["last","上月"]].map(([value,label])=><Button key={value} type="button" variant="ghost" size="sm" disabled={disabled} onClick={()=>preset(value)}>{t(label)}</Button>)}</div><div className="flex flex-wrap items-end gap-3"><label className="text-sm">{t("开始日期")}<Input type="date" value={days.from} onChange={e=>setDays({...days,from:e.target.value})} required disabled={disabled}/></label><label className="text-sm">{t("结束日期")}<Input type="date" value={days.to} onChange={e=>setDays({...days,to:e.target.value})} required disabled={disabled}/></label><Button type="submit" disabled={disabled}>{t(label)}</Button></div><p className="text-xs text-ink-mute">{t("日期按当前设备时区统计，包含结束当天；今天截至查询时刻。")}</p>{error&&<p role="alert" className="text-sm text-danger">{t(error)}</p>}</form>;
}
