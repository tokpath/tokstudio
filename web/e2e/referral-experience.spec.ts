import { expect, test } from "@playwright/test";
import { mockViewer } from "./mock-viewer";
import { createServer, type Server } from "node:http";

let catalogServer: Server;
test.beforeAll(async()=>{
  // RSC fetches cannot be intercepted by page.route. Only the exact model lookup
  // succeeds; the existing unavailable-home check remains a genuine failure.
  catalogServer=createServer((request,response)=>{
    const url=new URL(request.url || "/","http://127.0.0.1:8080");
    response.setHeader("Content-Type","application/json");
    if(url.pathname==="/v1/public/models" && url.searchParams.get("id")==="echo"){
      response.end(JSON.stringify({items:[{id:"echo",vendor:"Fixture",display_name:"Browser echo",kind:"text",status:"available",capabilities:{supported_endpoints:["/v1/chat/completions"]}}]}));return;
    }
    if(url.pathname==="/v1/public/models"){response.statusCode=503;response.end("{}");return;}
    if(url.pathname==="/v1/public/docs-context" && url.searchParams.get("model")==="echo"){
      response.end(JSON.stringify({model:"echo",api_base_url:"http://127.0.0.1:3000",supported_endpoints:["/v1/chat/completions"],examples:{"/v1/chat/completions":{curl:"curl http://127.0.0.1:3000/v1/chat/completions",python:"import urllib.request",node:"await fetch()"}}}));return;
    }
    if(url.pathname==="/v1/me" || url.pathname==="/v1/partner/me"){response.statusCode=401;response.end("{}");return;}
    response.statusCode=503;response.end(JSON.stringify({error:{message:"Fixture does not serve this endpoint"}}));
  });
  await new Promise<void>(resolve=>catalogServer.listen(8080,"127.0.0.1",resolve));
});
test.afterAll(async()=>{await new Promise<void>((resolve,reject)=>catalogServer.close(error=>error ? reject(error):resolve()));});

test("public API task keeps invitation and unavailable catalog has no sample models",async({page})=>{
  await page.route("**/v1/me",route=>route.fulfill({status:401,json:{error:{message:"signed out"}}}));
  await page.route("**/v1/partner/me",route=>route.fulfill({status:401,json:{}}));
  await page.route("**/v1/auth/google/status",route=>route.fulfill({json:{available:false}}));
  await page.goto("/?promotion_code=THU123");
  const create=page.getByRole("link",{name:"创建 API Key",exact:true}).first();
  await expect(create).toHaveAttribute("href","/login?next=%2Fapp%2Fkeys&promotion_code=THU123");
  await expect(page.getByText("模型目录暂时加载失败，当前无法确认可用模型。")).toBeVisible();
  await create.click();
  await expect(page.getByLabel("推广码",{exact:true})).toHaveValue("THU123");
  await expect(page.getByRole("button",{name:"使用 Google 登录"})).toHaveCount(0);
  await expect(page.getByRole("button",{name:"注册",exact:true})).toBeVisible();
});

test("ordinary API account has canonical invitation gifts progress and personal income",async({page})=>{
  await mockViewer(page,{roles:["end_user"]});
  await page.route("**/v1/me/referral**",route=>route.fulfill({json:{item:{codes:["THU123"],code_links:[{code:"THU123",share_url:"https://brand.example/login?promotion_code=THU123"}],can_create:true,invited_count:7,can_commission:false,professional_customers:false,rules:{spend_minor:10000000,topup_minor:5000000,gift_minor:1000000},progress:{spend_minor:4000000,largest_topup_minor:2000000,gift_granted_minor:3000000,gift_remaining_minor:1000000},summary:{earned_minor:4500000,frozen_minor:1500000,available_minor:3000000,held_minor:0,settled_minor:0,paid_minor:0,reversed_minor:0},rewards:[],settlements:[],pagination:{page:1,page_size:25,rewards_total:0,settlements_total:0}}}}));
  await page.goto("/app/referral");
  await expect(page.getByLabel("推广链接",{exact:true})).toHaveValue("https://brand.example/login?promotion_code=THU123");
  await expect(page.getByText("已直接邀请 7 人注册")).toBeVisible();
  await expect(page.getByRole("progressbar",{name:"本人累计已确认 API 消费"})).toHaveAttribute("value","4000000");
  await expect(page.getByRole("link",{name:"个人佣金",exact:true})).toBeVisible();
  await expect(page.getByRole("link",{name:"推广客户",exact:true})).toHaveCount(0);
  await page.screenshot({path:"test-results/t10-referral.png",fullPage:true});
  await page.getByRole("link",{name:"个人佣金",exact:true}).click();
  await expect(page.getByText("暂无佣金记录",{exact:true})).toBeVisible();
  await page.goto("/partner/settlements");
  await expect(page).toHaveURL(/\/app\/referral\?tab=settlements$/);
  await expect(page.getByText("暂无结算单",{exact:true})).toBeVisible();
});

test("purchase lost response resumes its original operation across reload",async({page})=>{
  await mockViewer(page,{roles:["end_user"]});
  await page.route("**/v1/me/plans",route=>route.fulfill({json:{items:[{id:"monthly",name:"Browser monthly",price_minor:2000000,currency:"USD",billing_period:"monthly",auto_renew_allowed:true,items:[{unit_type:"usd_credit",included_amount:3000000,expires_in_seconds:2592000}]}]}}));
  await page.route("**/v1/me/entitlements",route=>route.fulfill({json:{items:[]}}));
  await page.route("**/v1/payments/checkout",route=>route.fulfill({json:{item:{methods:[{adapter:"stripe",display_name:"Stripe",auto_renew_supported:true}]}}}));
  await page.route("**/v1/me/subscription-purchases/**",route=>route.fulfill({json:{item:{id:"browser-order",status:"pending"}}}));
  const ids:string[]=[];const bodies:unknown[]=[];
  await page.route("**/v1/me/subscriptions",async route=>{
    ids.push(route.request().headers()["idempotency-key"]);bodies.push(route.request().postDataJSON());
    if(ids.length===1){await route.abort("failed");return;}
    await route.fulfill({status:201,json:{checkout:{order:{id:"browser-order",status:"pending",amount_minor:2000000,currency:"USD"},sandbox:true}}});
  });
  await page.goto("/app/plans?plan=monthly&next=%2Fmodels%2Fecho%3Ftab%3Dagent");
  await expect(page.getByRole("checkbox")).toHaveCount(0);
  await expect(page.getByRole("link",{name:"返回原任务"})).toHaveAttribute("href","/models/echo?tab=agent");
  await page.screenshot({path:"test-results/t10-purchase.png",fullPage:true});
  await page.getByRole("button",{name:"订阅",exact:true}).click();
  await expect(page.getByRole("button",{name:"继续原购买"})).toBeVisible();
  await page.reload();
  await page.getByRole("button",{name:"继续原购买"}).click();
  await expect(page.getByText("browser-order",{exact:true})).toBeVisible();
  expect(ids).toHaveLength(2);expect(ids[1]).toBe(ids[0]);expect(bodies[1]).toEqual(bodies[0]);
  expect(bodies[0]).toEqual({plan_id:"monthly",adapter:"stripe",auto_renew:false});
});

test("OAuth denial recovers server invitation and next when browser intent is absent",async({page})=>{
  await page.route("**/v1/me",route=>route.fulfill({status:401,json:{}}));
  await page.route("**/v1/partner/me",route=>route.fulfill({status:401,json:{}}));
  await page.route("**/v1/auth/google/status",route=>route.fulfill({json:{available:false}}));
  await page.route("**/v1/auth/google/callback",route=>route.fulfill({status:400,headers:{"X-Login-Next":"/app/plans?plan=monthly","X-Promotion-Code":"THU123"},json:{error:{message:"denied"}}}));
  await page.goto("/login/oauth/google?state=synthetic-state&error=access_denied");
  await expect(page).toHaveURL(/\/login\?oauth_error=/);
  const url=new URL(page.url());expect(url.searchParams.get("next")).toBe("/app/plans?plan=monthly");expect(url.searchParams.get("promotion_code")).toBe("THU123");
  await expect(page.getByLabel("推广码",{exact:true})).toHaveValue("THU123");
});

test("home invitation survives public docs and model browsing with the original model task",async({page})=>{
  await page.route("**/v1/me",route=>route.fulfill({status:401,json:{}}));
  await page.route("**/v1/partner/me",route=>route.fulfill({status:401,json:{}}));
  await page.route("**/v1/auth/google/status",route=>route.fulfill({json:{available:false}}));
  await page.goto("/?promo=THU123");
  const docs=page.locator('a[href="/docs?promotion_code=THU123"]:visible').first();
  await docs.click();
  await expect(page).toHaveURL(/\/docs\?promotion_code=THU123$/);
  // The visitor can open a clean model URL after reading documentation. A
  // stored same-brand intent survives even when this page's CTA lacks the code.
  await page.goto("/models/echo");
  await expect(page.getByRole("heading",{name:"Browser echo",exact:true})).toBeVisible();
  await page.getByRole("tab",{name:"调用协议",exact:true}).click();
  await expect(page.getByTestId("model-protocol-example")).toBeVisible();
  const start=page.getByRole("link",{name:"创建 Key",exact:true});
  await start.click();
  await expect(page).toHaveURL(/\/login\?next=/);
  const keyTarget=new URL(new URL(page.url()).searchParams.get("next")!,"https://brand.test");
  expect(keyTarget.pathname).toBe("/app/keys");expect(keyTarget.searchParams.get("create")).toBe("1");expect(keyTarget.searchParams.get("model")).toBe("echo");expect(keyTarget.searchParams.get("tab")).toBe("protocol");
  const original=new URL(keyTarget.searchParams.get("return_to")!,"https://brand.test");expect(original.pathname).toBe("/models/echo");expect(original.searchParams.get("tab")).toBe("protocol");
  await expect(page.getByLabel("推广码",{exact:true})).toHaveValue("THU123");
  await expect(page.getByRole("button",{name:"注册",exact:true})).toBeVisible();
});
