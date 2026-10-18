// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { afterEach, expect, it, vi } from "vitest";
import { I18nProvider, useI18n } from "@/components/i18n";
import { translate, type Locale } from "@/lib/i18n";
import { FieldHelp } from "./field-help";
import { OperationForm } from "./operation-form";
import { configDefaults, configurationSchema } from "./operation-config";
import type { Entity, Operation } from "@/lib/contracts";
const help = "例如：账号有 100 ATG，给某个 Key 设置每月 20 ATG 预算，该 Key 当月最多使用 20 ATG；创建时不会划走这 20 ATG，其他 Key 仍共享账号可用额度。";
const response = (data: unknown) => new Response(JSON.stringify(data), { status: 200 });
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
function Locales() { const { setLocale } = useI18n(); return <>{(["zh", "en", "ja", "ru"] as Locale[]).map(locale => <button key={locale} onClick={() => setLocale(locale)}>{locale}</button>)}</>; }
function HelpForm({ submit, click }: { submit: () => void; click: () => void }) { const { t } = useI18n(); return <form onClick={click} onSubmit={event => { event.preventDefault(); submit(); }}><label>{t("名称")}<input defaultValue="authored-name" /></label><FieldHelp title={t("ATG 预算上限")}><p>{t(help)}</p></FieldHelp><button type="submit">submit</button></form>; }
it.each(["zh", "en", "ja", "ru"] as Locale[])("opens help without submitting, closes with Escape and restores focus in %s", async locale => {
 const submit=vi.fn(),click=vi.fn();render(<I18nProvider initialLocale={locale}><HelpForm submit={submit} click={click}/></I18nProvider>);
 const title=translate(locale,"ATG 预算上限"),trigger=screen.getByRole("button",{name:translate(locale,"了解{title}",{title})});
 trigger.focus();fireEvent.click(trigger);expect(await screen.findByRole("dialog",{name:title})).toBeTruthy();expect(screen.getByText(translate(locale,help))).toBeTruthy();
 expect(submit).not.toHaveBeenCalled();expect(click).not.toHaveBeenCalled();fireEvent.keyDown(document.activeElement||document.body,{key:"Escape"});
 await waitFor(()=>expect(screen.queryByRole("dialog")).toBeNull());await waitFor(()=>expect(document.activeElement).toBe(trigger));
 expect((screen.getByLabelText(translate(locale,"名称"))as HTMLInputElement).value).toBe("authored-name");expect(submit).not.toHaveBeenCalled();
});
function show(operation:Operation,row?:Entity){const cache=new QueryClient({defaultOptions:{queries:{retry:false}}});return render(<I18nProvider><Locales/><QueryClientProvider client={cache}><OperationForm operation={operation} operatorID="operator-one" row={row}/></QueryClientProvider></I18nProvider>)}
const route:Operation={name:"route",label:"保存路由",method:"POST",path:"/api/v1/operators/operator-one/routes",fields:[{name:"configuration",label:"配置 JSON",kind:"json",required:true}]};
it("generates route and price IDs and leaves the public model ID for an author",()=>{
 const price=configurationSchema({...route,path:"/api/v1/operators/operator-one/pricing",name:"price"})!,schema=configurationSchema(route)!,catalog=configurationSchema({...route,path:"/api/v1/operators/operator-one/catalog"})!;
 const first=configDefaults(schema)as{upstream:{id:string}},second=configDefaults(schema)as{upstream:{id:string}};
 expect(first.upstream.id).toMatch(/^[\da-f]{8}-(?:[\da-f]{4}-){3}[\da-f]{12}$/i);expect(second.upstream.id).not.toBe(first.upstream.id);
 expect(configDefaults(price)).toMatchObject({price:{id:expect.stringMatching(/^[\da-f-]{36}$/)}});expect(configDefaults(catalog)).toMatchObject({model:{id:""}});
});
it("retains generated route IDs through input and locale changes and preserves an existing route ID",()=>{
 vi.stubGlobal("fetch",vi.fn(async()=>response({items:[]})));const view=show(route),input=screen.getByLabelText("完整技术配置 JSON")as HTMLTextAreaElement,id=JSON.parse(input.value).upstream.id;expect(screen.queryByRole("textbox",{name:/^路由标识/})).toBeNull();
 expect(id).toMatch(/^[\da-f-]{36}$/);fireEvent.change(screen.getByRole("textbox",{name:/^供应商协议名称/}),{target:{value:"provider-protocol"}});fireEvent.click(screen.getByRole("button",{name:"ru",exact:true}));expect(JSON.parse(input.value).upstream.id).toBe(id);view.unmount();
 show(route,{id:"route-existing",version:7,values:{provider:"existing-protocol",id:"stale-value"}});expect(JSON.parse((screen.getByLabelText("完整技术配置 JSON")as HTMLTextAreaElement).value).upstream.id).toBe("route-existing");
});
it("opening help does not unlock or resubmit an unresolved command",async()=>{
 const calls:RequestInit[]=[];vi.stubGlobal("fetch",vi.fn(async(_path:RequestInfo|URL,init?:RequestInit)=>{calls.push(init!);if(calls.length===1)throw new TypeError("network lost");return response({id:"policy",status:"saved"})}));
 show({name:"policy",label:"保存比例",method:"POST",path:"/api/v1/operators/operator-one/commissions",fields:[{name:"direct_rate",label:"直接比例（0到1）",kind:"decimal",default_value:"0.050000000000000001",required:true}]});
 fireEvent.click(screen.getByRole("button",{name:"保存比例",exact:true}));await screen.findByRole("button",{name:"重试本次操作"});const input=screen.getByRole("textbox",{name:/^直接比例/})as HTMLInputElement;expect(input.disabled).toBe(true);
 fireEvent.click(screen.getByRole("button",{name:"了解直接比例（0到1）"}));await screen.findByRole("dialog");expect(calls).toHaveLength(1);expect(input.disabled).toBe(true);expect(input.value).toBe("0.050000000000000001");
 fireEvent.keyDown(document.activeElement||document.body,{key:"Escape"});await waitFor(()=>expect(screen.queryByRole("dialog")).toBeNull());fireEvent.click(screen.getByRole("button",{name:"重试本次操作"}));await waitFor(()=>expect(calls).toHaveLength(2));
 expect(calls[1].body).toBe(calls[0].body);expect(new Headers(calls[1].headers).get("Idempotency-Key")).toBe(new Headers(calls[0].headers).get("Idempotency-Key"));
});
