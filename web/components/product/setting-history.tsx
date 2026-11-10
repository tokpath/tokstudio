"use client";
import {useI18n} from "@/components/i18n";
import {When} from "./shared";
export function SettingHistory({value}:{value:unknown}) {
 const {t}=useI18n();
 const rows=Array.isArray(value)?value.filter((v):v is {actor:string;at:string;summary:string[]}=>!!v&&typeof v==="object"&&typeof v.actor==="string"&&typeof v.at==="string"&&Array.isArray(v.summary)&&v.summary.every((part:unknown)=>typeof part==="string")):[];
 if(!rows.length)return null;
 return <details className="mt-5 border-t border-hairline pt-4"><summary className="cursor-pointer text-sm text-ink-secondary">{t("最近变更")}</summary><ul className="mt-3 space-y-3">{rows.map((row,i)=><li key={i} className="text-sm"><p>{row.summary.map(part=>t(part)).join(" · ")}</p><p className="mt-1 text-xs text-ink-mute">{row.actor || t("未记录")} · <When value={row.at}/></p></li>)}</ul></details>;
}
