/** @vitest-environment jsdom */
import {cleanup, fireEvent, render, screen, waitFor} from "@testing-library/react";
import {afterEach, beforeEach, describe, expect, it, vi} from "vitest";
import {ModelUsagePanel} from "@/components/model-usage-panel";
import {withZh} from "./test-i18n";

const model = {id: "openai/test-text", display_name: "Text", vendor: "openai", kind: "text"};
const key = {id: "key_a", name: "Work", prefix: "thk_a", key: "thk_secret_a", status: "active", model_mode: "all"};
function docs(path = "/v1/chat/completions", base = "https://brand.example") {
 return {model: model.id, api_base_url: base, supported_endpoints: [path], examples: {[path]: {curl: `curl ${base}${path} -H "Authorization: Bearer \${TOKENHUB_API_KEY}"`, python: "import urllib.request", node: "await fetch()"}}};
}
function stubFetch(result: {ok: boolean; error?: {code: string; message: string}} = {ok: true}, path = "/v1/chat/completions", base = "https://brand.example") {
 const mock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
  if (String(input).includes("docs-context")) return {ok: true, json: async () => docs(path, base)};
  if (String(input).includes("/v1/me/api-keys")) return {ok: true, json: async () => ({items: [key, {...key,id: "key_b", name: "B",key: "thk_secret_b"}]})};
  return {ok: result.ok, json: async () => ({request_id: "req_platform", error: result.error})};
 });
 vi.stubGlobal("fetch", mock);return mock;
}
async function show(path = "/v1/chat/completions", result: {ok: boolean; error?: {code: string; message: string}} = {ok: true}, base = "https://brand.example") {
 const mock = stubFetch(result, path, base);
 render(withZh(<ModelUsagePanel model={model} keyID={key.id}/>));
 await waitFor(() => expect(screen.getByTestId("model-protocol-example")).toBeTruthy());await screen.findByRole("option",{name:/Work/});return mock;
}
describe("model instructions and optional test", () => {
 beforeEach(() => {window.history.replaceState(null,"","/app/docs?model=openai%2Ftest-text&key_id=key_a");});
 afterEach(() => {cleanup();vi.unstubAllGlobals();});
 it.each(["https://brand.example", "http://localhost:9080"])("uses real branded %s and keeps secrets out of URLs/examples", async base => {
  const mock = await show("/v1/chat/completions",{ok:true},base);
  expect(screen.getByTestId("model-protocol-example").textContent).toContain(base);
  expect(screen.getByTestId("model-protocol-example").textContent).not.toContain(key.key);
  expect(mock.mock.calls.some(([,init]) => init?.method === "POST")).toBe(false);
  fireEvent.click(screen.getByRole("tab",{name:"Agent 配置"}));
  expect(screen.getByText(/API Provider 选择 OpenAI Compatible/)).toBeTruthy();expect(screen.getByText(`${base}/v1`)).toBeTruthy();
  expect(window.location.search).toContain("key_id=key_a");expect(window.location.href).not.toContain(key.key);
  const wallet=screen.getByRole("link",{name:"账户余额 / 充值"}).getAttribute("href")!;
  expect(decodeURIComponent(wallet)).toContain("key_id=key_a");expect(decodeURIComponent(wallet)).toContain("model=openai%2Ftest-text");
 });
 it.each(["/v1/chat/completions","/v1/responses","/v1/messages"])("tests selected protocol %s with selected bearer only after explicit click",async path=>{
  const mock=await show(path);
  expect(screen.getByText(/按当前模型价格计费/)).toBeTruthy();fireEvent.click(screen.getByRole("button",{name:"发送站内测试请求"}));
  await waitFor(()=>expect(screen.getByTestId("model-verify-status").textContent).toContain("站内调用成功"));
  const post=mock.mock.calls.find(([,init])=>init?.method==="POST")!;expect(String(post[0])).toBe(`https://brand.example${path}`);expect(post[1]?.headers).toMatchObject({Authorization:`Bearer ${key.key}`});
  const body=JSON.parse(String(post[1]?.body));expect(body.model).toBe(model.id);expect(body[path==="/v1/responses"?"max_output_tokens":"max_tokens"]).toBe(32);
  expect(screen.getByText("req_platform")).toBeTruthy();
 });
 it.each([
  {code:"key_expired",label:"编辑限制"}, {code:"model_not_allowed",label:"编辑限制"}, {code:"key_budget_exceeded",label:"编辑限制"},
  {code:"insufficient_balance",label:"账户余额 / 充值"}, {code:"rate_limited",label:"查看请求"}, {code:"request_outcome_unknown",label:"查看请求"},
 ])("keeps $code distinct with a recovery action",async({code,label})=>{
  stubFetch({ok:false,error:{code,message:`failure:${code}`}});render(withZh(<ModelUsagePanel model={model} keyID={key.id}/>));
  await waitFor(()=>expect(screen.getByTestId("model-protocol-example")).toBeTruthy());await screen.findByRole("option",{name:/Work/});fireEvent.click(screen.getByRole("button",{name:"发送站内测试请求"}));
  await waitFor(()=>expect(screen.getByTestId("model-verify-status").textContent).toContain(`failure:${code}`));
  expect(screen.getByTestId("model-verify-status").querySelector("a")?.textContent).toBe(label);expect(screen.queryByText(/站内调用成功/)).toBeNull();
 });
 it("does not invent a hostname or protocol after docs failure",async()=>{
  vi.stubGlobal("fetch",vi.fn(async input=>({ok:!String(input).includes("docs-context"),json:async()=>String(input).includes("docs-context")?{error:{message:"unavailable"}}:{items:[key]}})));
  render(withZh(<ModelUsagePanel model={model} keyID={key.id}/>));await waitFor(()=>expect(screen.getByText("unavailable")).toBeTruthy());expect(screen.queryByTestId("model-protocol-example")).toBeNull();expect(screen.queryByRole("button",{name:"发送站内测试请求"})).toBeNull();
 });

 it("keeps model instructions readable when Key loading fails and retries the Key list",async()=>{
  let failed=true;
  vi.stubGlobal("fetch",vi.fn(async input=>String(input).includes("docs-context")?{ok:true,json:async()=>docs()}:failed?{ok:false,status:500,json:async()=>({error:{message:"Key list unavailable"}})}:{ok:true,json:async()=>({items:[key]})}));
  render(withZh(<ModelUsagePanel model={model} keyID={key.id}/>));
  await screen.findByText("Key list unavailable");expect(screen.getByTestId("model-protocol-example")).toBeTruthy();
  expect(screen.getByLabelText("当前 Key")).toHaveProperty("disabled",true);
  expect(screen.queryByText("key_a")).toBeNull();
  failed=false;fireEvent.click(screen.getByRole("button",{name:"重新读取"}));
  await screen.findByRole("option",{name:/Work/});expect(screen.queryByText("Key list unavailable")).toBeNull();
 });

 it("returns parameter failures to the selected protocol and preserves context",async()=>{
  await show("/v1/chat/completions",{ok:false,error:{code:"invalid_request",message:"unsupported parameter"}});
  fireEvent.click(screen.getByRole("tab",{name:"Agent 配置"}));
  fireEvent.click(screen.getByRole("button",{name:"发送站内测试请求"}));
  const status=await screen.findByTestId("model-verify-status");
  const link=status.querySelector("a")!;expect(link.getAttribute("href")).toContain("tab=protocol");expect(link.getAttribute("href")).toContain("key_id=key_a");
  fireEvent.click(link);expect(screen.getByRole("tab",{name:"调用协议"}).getAttribute("aria-selected")).toBe("true");
 });
 it("never labels incomplete Responses/Messages adapters as working Codex/Claude Code configs",async()=>{
  await show();fireEvent.click(screen.getByRole("tab",{name:"Agent 配置"}));expect(screen.queryByRole("option",{name:/Codex/})).toBeNull();expect(screen.getByText(/尚缺 Responses 事件流/)).toBeTruthy();
  expect(screen.queryByRole("option",{name:/Claude Code/})).toBeNull();expect(screen.getByText(/尚缺完整内容块往返/)).toBeTruthy();
 });
 it("does not apply a late verification for A onto selected Key B",async()=>{
  let resolve!: (value: unknown)=>void;const pending=new Promise(res=>{resolve=res});const mock=stubFetch();mock.mockImplementation(async(input,init)=>{
   if(init?.method==="POST")return pending as never;
   return {ok:true,json:async()=>String(input).includes("docs-context")?docs():{items:[key,{...key,id:"key_b",name:"B",key:"thk_secret_b"}]}};
  });
  render(withZh(<ModelUsagePanel model={model} keyID={key.id}/>));await waitFor(()=>expect(screen.getByTestId("model-protocol-example")).toBeTruthy());await screen.findByRole("option",{name:/Work/});fireEvent.click(screen.getByRole("button",{name:"发送站内测试请求"}));
  fireEvent.change(screen.getByLabelText("当前 Key"),{target:{value:"key_b"}});resolve({ok:true,json:async()=>({request_id:"req_a"})});
  await waitFor(()=>expect(screen.getByLabelText("当前 Key")).toHaveProperty("value","key_b"));expect(screen.queryByTestId("model-verify-status")).toBeNull();expect(window.location.href).not.toContain(key.key);
 });
 it("restores the SDK language and protocol from the URL without dropping invitation or return context",async()=>{
  window.history.replaceState(null,"","/docs?model=openai%2Ftest-text&key_id=key_a&protocol=%2Fv1%2Fresponses&language=python_sdk&promo=invite-a&return_to=%2Fmodels%3Fq%3Dtest");
  const body=docs();body.supported_endpoints.push("/v1/responses");
  Object.assign(body.examples,{"/v1/responses":{curl:"curl https://brand.example/v1/responses",python_sdk:'OpenAI(base_url="https://brand.example/v1", max_retries=0)'}});
  vi.stubGlobal("fetch",vi.fn(async input=>({ok:true,json:async()=>String(input).includes("docs-context")?body:{items:[key]}})));
  render(withZh(<ModelUsagePanel model={model} keyID={key.id}/>));
  await waitFor(()=>expect(screen.getByTestId("model-protocol-example").textContent).toContain("max_retries=0"));
  expect(screen.getByLabelText("调用协议")).toHaveProperty("value","/v1/responses");
  const wallet=new URL(screen.getByRole("link",{name:"账户余额 / 充值"}).getAttribute("href")!,window.location.origin);
  const next=wallet.searchParams.get("next")!;expect(next.startsWith("/docs?")).toBe(true);expect(next).toContain("language=python_sdk");expect(next).toContain("promo=invite-a");expect(next).toContain("return_to=%2Fmodels%3Fq%3Dtest");
  expect(screen.getByTestId("model-key-environment").textContent).toBe("export TOKENHUB_API_KEY='PASTE_YOUR_KEY'");expect(document.body.textContent).not.toContain(key.key);
 });
 it("offers only generated SDK choices and preserves tool context across tabs",async()=>{
  const body=docs();Object.assign(body.examples["/v1/chat/completions"],{python_sdk:"from openai import OpenAI",node_sdk:'import OpenAI from "openai"',aider:"export OPENAI_API_BASE='https://brand.example/v1'\naider --model 'openai/openai/test-text'"});
  vi.stubGlobal("fetch",vi.fn(async input=>({ok:true,json:async()=>String(input).includes("docs-context")?body:{items:[key]}})));
  render(withZh(<ModelUsagePanel model={model} keyID={key.id}/>));await screen.findByTestId("model-protocol-example");
  fireEvent.click(screen.getByRole("button",{name:"Node.js · OpenAI SDK"}));expect(screen.getByTestId("model-protocol-example").textContent).toContain('import OpenAI');
  fireEvent.click(screen.getByRole("tab",{name:"Agent 配置"}));fireEvent.change(screen.getByLabelText("接入工具"),{target:{value:"aider"}});
  expect(screen.getByTestId("model-agent-config").textContent).toContain("openai/openai/test-text");expect(window.location.search).toContain("tool=aider");
  expect(screen.getByText(/先用 \/ask/)).toBeTruthy();expect(screen.getByText(/本页未执行外部 Agent/)).toBeTruthy();
  fireEvent.click(screen.getByRole("tab",{name:"调用协议"}));expect(screen.getByTestId("model-protocol-example").textContent).toContain('import OpenAI');expect(window.location.search).toContain("language=node_sdk");
 });
 it("falls back to HTTP when a copied SDK language is unavailable on Messages",async()=>{
  window.history.replaceState(null,"","/app/docs?language=node_sdk&protocol=%2Fv1%2Fmessages");await show("/v1/messages");
  expect(screen.queryByRole("button",{name:"Node.js · OpenAI SDK"})).toBeNull();expect(screen.getByTestId("model-protocol-example").textContent).toContain("/v1/messages");
  fireEvent.click(screen.getByRole("tab",{name:"Agent 配置"}));expect(screen.queryByTestId("model-agent-config")).toBeNull();expect(screen.queryByText("Base URL")).toBeNull();
 });
 it("uses the current estimate error and describes actual settlement rather than an absolute cap",async()=>{
  stubFetch({ok:false,error:{code:"price_estimate_unavailable",message:"cannot estimate"}});
  render(withZh(<ModelUsagePanel model={{...model,capabilities:{budget_estimate_supported:true}}} keyID={key.id}/>));
  await screen.findByRole("option",{name:/Work/});expect(screen.getByText(/单次消费可能超过 Key 上限/)).toBeTruthy();
  expect(screen.getByText(/充值不会重置已发生的累计消费/)).toBeTruthy();expect(document.body.textContent).not.toContain("key_budget_unbounded");
  fireEvent.click(screen.getByRole("button",{name:"发送站内测试请求"}));await waitFor(()=>expect(screen.getByTestId("model-verify-status").textContent).toContain("price_estimate_unavailable"));
  expect(screen.getByTestId("model-verify-status").textContent).not.toContain("取消");
 });
});
