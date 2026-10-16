"use client";
import { useQuery } from "@tanstack/react-query";
import { z } from "zod";
import { request } from "@/lib/api-client";
import { PaymentAction } from "./customer-plans";
import { useState,useDeferredValue, type FormEvent } from "react";
import { CommandResultSchema, Decimal, JSONValueSchema, type CommandField, type Entity, type Operation, type JSONValue } from "@/lib/contracts";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { ErrorPanel, Notice, useCommand } from "./shared";

function nested(value: unknown,path:string): unknown { return path.split(".").reduce<unknown>((current,key)=>current!==null&&typeof current==="object"?(current as Record<string,unknown>)[key]:undefined,value); }
function assign(target:Record<string,JSONValue>,path:string,value:JSONValue){const keys=path.split(".");if(keys.some(k=>["__proto__","prototype","constructor"].includes(k)))throw new Error("字段路径不合法");let current:Record<string,JSONValue>|JSONValue[]=target;keys.forEach((key,index)=>{const object=current as Record<string,JSONValue>;if(index===keys.length-1){object[key]=value;return}if(!object[key])object[key]=/^\d+$/.test(keys[index+1])?[]:{};current=object[key] as Record<string,JSONValue>})}
function fieldString(field:CommandField,value:unknown):string{if(value===undefined||value===null)return field.kind==="boolean"?"false":"";if(field.kind==="csv"&&Array.isArray(value))return value.join(",");if(field.kind==="datetime"&&typeof value==="string"){const date=new Date(value);if(!Number.isNaN(date.getTime()))return new Date(date.getTime()-date.getTimezoneOffset()*60000).toISOString().slice(0,16)}return typeof value==="object"?JSON.stringify(value,null,2):String(value)}
function initial(field: CommandField,row?:Entity):string{if((field.name==="expected_version"||field.name==="version")&&row?.version!==undefined)return String(row.version);return fieldString(field,nested(row?.values,field.name)??field.default_value)}

export function OperationForm({ operation, row, operatorID, onClose }: { operation: Operation; row?: Entity; operatorID?: string; onClose?: () => void }) {
  const path = operation.path.replaceAll("{id}", encodeURIComponent(row?.id || "")).replaceAll("{operator_id}", encodeURIComponent(operatorID || ""));
  const [values, setValues] = useState<Record<string, string>>(() => Object.fromEntries(operation.fields.map(field => [field.name, initial(field, row)]))), [files, setFiles] = useState<Record<string, File>>({}), [confirmed, setConfirmed] = useState(false), [validationError, setValidationError] = useState<string>();
  const command = useCommand(path, CommandResultSchema, operation.method);
  const scoped = operatorID ? path.startsWith(`/api/v1/operators/${encodeURIComponent(operatorID)}/`) : path.startsWith("/api/v1/me/");
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setValidationError(undefined);
    if (!scoped) { setValidationError("操作不属于当前经营空间，已阻止提交。"); return; }
    try {
      const payload: Record<string, JSONValue> = {};
      for (const field of operation.fields) {
        if(field.visible_when&&!field.visible_when.values.includes(values[field.visible_when.field]||""))continue;
        const value = values[field.name] || "";
        if (field.kind === "file") { if (field.required && !files[field.name]) throw new Error(`请上传${field.label}`); continue; }
        if (!value && !field.required) { if(field.kind==="csv")payload[field.name]=[];else if(field.name.startsWith("values.")||field.name==="second_review_above")assign(payload,field.name,null); continue; }
        if (!value && field.required) throw new Error(`请填写${field.label}`);
        if(field.kind==="csv")payload[field.name]=value.split(",").map(v=>v.trim()).filter(Boolean);
        else if (field.kind === "decimal") payload[field.name] = Decimal.parse(value);
        else if ((field.kind === "integer"||field.name==="expected_version"||field.name==="version")) { if (!/^\d+$/.test(value) || !Number.isSafeInteger(Number(value))) throw new Error(`${field.label}应为非负整数`); payload[field.name] = Number(value); }
        else if (field.kind === "boolean" || (field.kind==="hidden"&&typeof field.default_value==="boolean")) payload[field.name] = value === "true";
        else if (field.kind === "json") payload[field.name] = JSONValueSchema.parse(JSON.parse(value));
        else if (field.kind === "datetime") { const date = new Date(value); if (Number.isNaN(date.getTime())) throw new Error(`${field.label}时间无效`); payload[field.name] = date.toISOString(); }
        else payload[field.name] = value;
      }
      const structured:Record<string,JSONValue>={};for(const [name,value]of Object.entries(payload))assign(structured,name,value);for(const name of Object.keys(payload))delete payload[name];Object.assign(payload,structured);
      if (operation.include_version && row?.version !== undefined) payload.version = row.version;
      let completed;
      if (Object.keys(files).length) { const data = new FormData(); for (const [key, value] of Object.entries(payload)) data.append(key, typeof value === "string" ? value : JSON.stringify(value)); for (const [key, file] of Object.entries(files)) data.append(key, file); completed=await command.run(data); }
      else completed=await command.run(payload);
      void completed;
    } catch (error) { setValidationError(error instanceof Error ? error.message : "请检查输入"); }
  }
  return <Card className="my-5"><div className="mb-5 flex items-start justify-between gap-4"><div><h2 className="text-lg font-semibold">{operation.label}</h2>{operation.description && <p className="mt-2 text-sm leading-7 text-ink-secondary">{operation.description}</p>}{row && <p className="mt-2 break-all font-mono text-xs text-ink-mute">事项 {row.id}</p>}</div>{onClose && <Button type="button" variant="ghost" size="sm" onClick={onClose}>关闭</Button>}</div>
    <form onSubmit={submit} className="space-y-5"><div className="grid gap-5 md:grid-cols-2">{operation.fields.filter(field=>field.kind!=="hidden"&&(!field.visible_when||field.visible_when.values.includes(values[field.visible_when.field]||""))).map(field => <Field key={field.name} field={field} value={values[field.name] || ""} disabled={command.busy || command.unresolved || !!command.result} change={value => setValues(previous => ({ ...previous, [field.name]: value }))} onChoice={defaults=>setValues(previous=>{const next={...previous};for(const [name,value]of Object.entries(defaults)){const field=operation.fields.find(f=>f.name===name);if(field)next[name]=fieldString(field,value)}return next})} onFile={file => setFiles(previous => ({ ...previous, [field.name]: file }))} />)}</div>
      {operation.confirmation && <label className="flex items-start gap-3 text-sm leading-7"><input type="checkbox" required checked={confirmed} onChange={e => setConfirmed(e.target.checked)} className="mt-2" disabled={command.busy || command.unresolved || !!command.result} />{operation.confirmation}</label>}
      {validationError && <p role="alert" className="text-sm text-danger">{validationError}</p>}{!!command.error && <ErrorPanel error={command.error} />}
      {command.unresolved && <Notice>上次提交结果尚未核实，请查询记录或重试原操作。重试保留原字段和操作编号。<span className="block break-all font-mono text-xs">{command.actionKey}</span></Notice>}
      {command.result && <Notice>{command.result.message || "操作已受理。"}<span className="ml-2 font-mono text-xs">{command.result.id}</span><span className="ml-2">当前状态：{command.result.status}</span></Notice>}
      <Button type="submit" disabled={command.busy || !scoped || !!command.result}>{command.busy ? "处理中…" : command.unresolved ? "重试本次操作" : command.result?"本次操作已完成，关闭后查看最新记录":operation.label}</Button>
    </form>{command.result?.order&&<PaymentAction order={command.result.order}/>}
  </Card>;
}
function Field({ field, value, disabled, change, onFile,onChoice }: { field: CommandField; value: string; disabled: boolean; change: (value: string) => void; onFile: (value: File) => void;onChoice:(values:Record<string,unknown>)=>void }) {
  if(field.lookup_url)return <ReferenceField field={field} value={value} disabled={disabled} change={change} choose={onChoice}/>;
  const base = "mt-2 min-h-11 w-full rounded-control border border-hairline bg-canvas-raised px-3 py-2 text-sm";
  return <label className={`block text-sm ${["textarea", "json"].includes(field.kind) ? "md:col-span-2" : ""}`}>{field.label}{field.required && <span className="ml-1 text-danger">*</span>}
    {field.kind === "boolean" ? <span className="mt-3 flex items-center gap-2"><input type="checkbox" checked={value === "true"} disabled={disabled} onChange={event => change(event.target.checked ? "true" : "false")} />启用</span> : field.kind === "select" ? <select className={base} value={value} required={field.required} disabled={disabled} onChange={event => change(event.target.value)}><option value="">请选择</option>{field.options?.map(option => <option key={option.value} value={option.value}>{option.label}</option>)}</select> : ["textarea", "json"].includes(field.kind) ? <textarea className={`${base} min-h-28 ${field.kind === "json" ? "font-mono" : ""}`} value={value} required={field.required} disabled={disabled} onChange={event => change(event.target.value)} spellCheck={field.kind !== "json"} /> : field.kind === "file" ? <Input className="mt-2" type="file" required={field.required} disabled={disabled} onChange={event => { const file = event.target.files?.[0]; if (file) onFile(file); }} /> : <Input className="mt-2" type={field.kind === "email" ? "email" : field.kind === "password" ? "password" : field.kind === "datetime" ? "datetime-local" : "text"} inputMode={field.kind === "decimal" ? "decimal" : field.kind === "integer" ? "numeric" : undefined} autoComplete={field.kind === "password" ? "new-password" : "off"} value={value} required={field.required} disabled={disabled} onChange={event => change(event.target.value)} />}
    {field.help && <span className="mt-2 block text-xs leading-6 text-ink-mute">{field.help}</span>}
  </label>;
}

const ReferenceSchema=z.object({items:z.array(z.object({value:z.string(),label:z.string(),defaults:z.record(z.string(),z.unknown()).optional()})),next_cursor:z.string().optional()});
function ReferenceField({field,value,disabled,change,choose}:{field:CommandField;value:string;disabled:boolean;change:(value:string)=>void;choose:(values:Record<string,unknown>)=>void}){
 const [search,setSearch]=useState("");const deferred=useDeferredValue(search);const [cursor,setCursor]=useState("");const [selectedLabel,setSelectedLabel]=useState("");const path=field.lookup_url!;
 const data=useQuery({queryKey:["references",path,deferred,cursor],queryFn:({signal})=>request(`${path}?q=${encodeURIComponent(deferred)}&cursor=${encodeURIComponent(cursor)}`,ReferenceSchema,{signal})});
 return <fieldset className="min-w-0 text-sm"><legend>{field.label}{field.required&&<span className="ml-1 text-danger">*</span>}</legend><Input className="mt-2" aria-label={`搜索${field.label}`} placeholder="按名称或编号搜索" value={search} disabled={disabled} onChange={event=>{setSearch(event.target.value);setCursor("")}}/><select aria-label={field.label} className="mt-2 min-h-11 w-full min-w-0 rounded-control border border-hairline bg-canvas-raised px-3 py-2 text-sm" value={value} required={field.required} disabled={disabled||data.isPending} onChange={event=>{const next=event.target.value;change(next);const option=data.data?.items.find(item=>item.value===next);setSelectedLabel(option?.label||"");choose(option?.defaults||{expected_version:0})}}><option value="">{field.required?"请选择实际业务记录":"新建 / 不选择现有记录"}</option>{value&&!data.data?.items.some(item=>item.value===value)&&<option value={value}>{selectedLabel||value}</option>}{data.data?.items.map(item=><option value={item.value} key={item.value}>{item.label}</option>)}</select>{data.isError&&<div className="mt-2"><ErrorPanel error={data.error} retry={()=>void data.refetch()} compact/></div>}{data.data?.next_cursor&&<Button variant="outline" size="sm" type="button" className="mt-2" disabled={disabled} onClick={()=>setCursor(data.data!.next_cursor!)}>更多记录</Button>}{field.help&&<p className="mt-2 text-xs leading-6 text-ink-mute">{field.help}</p>}</fieldset>
}
