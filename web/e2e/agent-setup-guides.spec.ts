import {expect, test} from "@playwright/test";
import {createServer, type Server} from "node:http";

let server: Server;
const model="vendor/guide-text";
const api="https://api.guide-brand.example";
const chat="/v1/chat/completions";
const responses="/v1/responses";
test.beforeAll(async()=>{
  server=createServer((request,response)=>{
    const url=new URL(request.url||"/","http://127.0.0.1:8080");response.setHeader("Content-Type","application/json");
    if(url.pathname==="/v1/public/models"){
      response.end(JSON.stringify({items:[{id:model,vendor:"Fixture",display_name:"Guide model",kind:"text",status:"active",context_length:8192,max_completion_tokens:2048,capabilities:{supported_endpoints:[chat,responses],budget_estimate_supported:true}}]}));return;
    }
    if(url.pathname==="/v1/public/docs-context"){
      response.end(JSON.stringify({model,api_base_url:api,supported_endpoints:[chat,responses],examples:{
        [chat]:{curl:`curl ${api}${chat}`,python:"import urllib.request",node:"await fetch()",python_sdk:`OpenAI(base_url="${api}/v1", max_retries=0)`,node_sdk:`new OpenAI({baseURL: "${api}/v1", maxRetries: 0})`,aider:`export TOKENHUB_API_KEY='PASTE_YOUR_KEY'\nexport OPENAI_API_BASE='${api}/v1'\naider --model 'openai/${model}'`},
        [responses]:{curl:`curl ${api}${responses}`,python_sdk:`OpenAI(base_url="${api}/v1").responses.create(input="hi")`},
      }}));return;
    }
    if(url.pathname==="/v1/me" || url.pathname==="/v1/me/api-keys"){response.statusCode=401;response.end("{}");return;}
    response.end("{}");
  });
  await new Promise<void>(resolve=>server.listen(8080,"127.0.0.1",resolve));
});
test.afterAll(async()=>{await new Promise<void>((resolve,reject)=>server.close(error=>error?reject(error):resolve()));});

test("public Agent and code setup preserves brand, tool, protocol and invitation across reload",async({page,context})=>{
  await context.grantPermissions(["clipboard-read","clipboard-write"]);
  const post:string[]=[];page.on("request",request=>{if(request.method()==="POST")post.push(request.url());});
  await page.goto(`/docs/integrations?model=${encodeURIComponent(model)}&promo=guide-invite&return_to=%2Fmodels%3Fq%3Dguide`);
  await expect(page.getByRole("heading",{name:"Guide model"})).toBeVisible();
  await expect(page.locator("dd").filter({hasText:`${api}/v1`})).toBeVisible();
  await page.getByLabel("接入工具").selectOption("aider");
  await expect(page.getByTestId("model-agent-config")).toContainText(`openai/${model}`);
  await page.getByRole("button",{name:"复制",exact:true}).click();
  expect(await page.evaluate(()=>navigator.clipboard.readText())).toContain(`${api}/v1`);
  await page.reload();await expect(page.getByLabel("接入工具")).toHaveValue("aider");
  await expect(page.getByTestId("model-agent-config")).toContainText("PASTE_YOUR_KEY");
  await page.getByRole("tab",{name:"调用协议",exact:true}).click();
  await page.getByLabel("调用协议",{exact:true}).selectOption(responses);
  await page.getByRole("button",{name:"Python · OpenAI SDK",exact:true}).click();
  await page.reload();await expect(page.getByLabel("调用协议",{exact:true})).toHaveValue(responses);
  await expect(page.getByTestId("model-protocol-example")).toContainText(`${api}/v1`);
  await expect(page.getByTestId("model-protocol-example")).toContainText("responses.create");
  const walletHref=await page.getByRole("link",{name:"账户余额 / 充值",exact:true}).getAttribute("href");
  expect(walletHref).toContain("/app/wallet?next=");
  const next=new URL(walletHref!,"http://example.test").searchParams.get("next")!;
  const back=new URL(next,"http://example.test");expect(back.pathname).toBe("/docs/integrations");expect(back.searchParams.get("tool")).toBe("aider");expect(back.searchParams.get("language")).toBe("python_sdk");expect(back.searchParams.get("promo")).toBe("guide-invite");expect(back.searchParams.get("return_to")).toBe("/models?q=guide");
  await page.getByRole("tab",{name:"Agent 配置",exact:true}).click();
  await page.getByText("Codex / Claude Code",{exact:false}).click();
  await expect(page.getByText(/尚缺 Responses 事件流/)).toBeVisible();await expect(page.getByText(/尚缺完整内容块往返/)).toBeVisible();
  expect(post).toEqual([]);
  await page.evaluate(()=>window.scrollTo({top:0,behavior:"instant"}));
  await page.screenshot({path:"test-results/agent-setup-guides.png",fullPage:true});
});
