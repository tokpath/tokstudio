import { test, expect, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";
const ready = process.env.TOKENHUB_EXIT_ACCEPTANCE_FILE;
test.skip(!ready, "Requires the isolated exit acceptance application");
test.use({ actionTimeout: 15_000 });
async function login(page: Page, email: string, password: string, staff=true) {
  await page.goto("/login");
  await page.getByLabel("邮箱", {exact:true}).fill(email);
  await page.getByLabel("密码", {exact:true}).fill(password);
  await page.getByRole("button", {name:"登录",exact:true}).click();
  await expect(page).toHaveURL(staff?/\/manage\//:/\/workspace/);
}
test("platform OEM channel and customer complete simplified takeover in the live app", async({browser},info)=>{
  test.setTimeout(150_000);
  const f=JSON.parse(readFileSync(ready!,"utf8"));
  const staffContext=await browser.newContext({baseURL:process.env.TOKENHUB_WEB_ORIGIN,locale:"zh-CN",viewport:{width:1440,height:1000}});
  const customerOrigin=new URL(process.env.TOKENHUB_WEB_ORIGIN!);customerOrigin.hostname="exit.localhost";
  const customerContext=await browser.newContext({baseURL:customerOrigin.origin,locale:"zh-CN",viewport:{width:1280,height:1000}});
  const page=await staffContext.newPage(),customer=await customerContext.newPage();
  try{
    await login(page,f.owner_email,f.owner_password);
    await page.goto(`/manage/${f.platform}/exits`);
    await expect(page.getByRole("heading",{name:"停业与接管",exact:true})).toBeVisible();
    await expect(page.getByRole("button",{name:"告知平台停止经营",exact:true})).toHaveCount(0);
    await page.screenshot({path:info.outputPath("platform-exits.png"),fullPage:true});
    await page.goto(`/manage/${f.oem}/exits`);
    await page.getByRole("button",{name:"告知平台停止经营",exact:true}).click();
    let panel=page.getByRole("dialog");
    await expect(panel.getByRole("textbox")).toHaveCount(1);
    await panel.getByRole("textbox").fill("测试停业告知：客户责任由本品牌处理");
    if(await panel.getByRole("checkbox").count()) await panel.getByRole("checkbox").check();
    await panel.getByRole("button",{name:"告知平台停止经营",exact:true}).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(page.getByText("OEM 停业告知",{exact:false})).toBeVisible();
    // The brand initiates takeover directly: the channel has never submitted an exit notice.
    await page.goto(`/manage/${f.oem}/takeovers`);
    await page.getByRole("button",{name:"接管渠道",exact:true}).click();
    panel=page.getByRole("dialog");
    await panel.getByRole("combobox",{name:"直属渠道",exact:true}).selectOption(f.channel);
    await panel.getByRole("checkbox").check();
    await panel.getByRole("button",{name:"开始准备",exact:true}).click();
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect(page.getByRole("button",{name:"确认接管",exact:true})).toBeEnabled();
    await expect(page.getByText("渠道月套餐",{exact:true})).toBeVisible();
    await expect(page.getByText("渠道不限时套餐",{exact:true})).toBeVisible();
    for(const locale of ["zh","en","ja","ru"]){
      await page.getByTestId("language-selector").selectOption(locale);
      await expect(page.getByTestId("language-selector")).toHaveValue(locale);
      await page.screenshot({path:info.outputPath(`takeover-${locale}.png`),fullPage:true});
      expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
    }
    await page.getByTestId("language-selector").selectOption("zh");
    await page.setViewportSize({width:390,height:844});
    await page.screenshot({path:info.outputPath("takeover-mobile.png"),fullPage:true});
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
    await page.setViewportSize({width:1440,height:1000});
    await page.goto(`/manage/${f.channel}/exits`);
    await expect(page).toHaveURL(new RegExp(`/manage/${f.channel}/takeovers$`));
    await expect(page.getByText("正在由星河品牌接管，新增经营已暂停",{exact:true})).toBeVisible();
    await expect(page.getByRole("button",{name:"通知品牌办理退出",exact:true})).toHaveCount(0);
    await page.goto(`/manage/${f.oem}/takeovers`);
    await page.getByRole("button",{name:"确认接管",exact:true}).click();
    panel=page.getByRole("dialog");await expect(panel.getByRole("textbox")).toHaveCount(0);
    await panel.getByRole("checkbox").check();await panel.getByRole("button",{name:"确认提交",exact:true}).click();
    await expect(page.getByText("接管完成",{exact:true})).toBeVisible();
    await expect(page.getByRole("button",{name:"登记实际收付款",exact:true})).toHaveCount(0);
    await page.screenshot({path:info.outputPath("takeover-completed.png"),fullPage:true});
    await page.goto(`/manage/${f.oem}/plans`);
    await expect(page.getByText("来自渠道：星河渠道",{exact:true}).first()).toBeVisible();
    await page.screenshot({path:info.outputPath("copied-packages.png"),fullPage:true});
    await page.goto(`/manage/${f.channel}/takeovers`);
    await expect(page.getByText("接管完成",{exact:true})).toBeVisible();
    await expect(page.getByRole("button",{name:"接管渠道",exact:true})).toHaveCount(0);
    await expect(page.getByText("已由星河品牌接管，渠道经营已结束",{exact:true})).toBeVisible();
    await expect(page.getByRole("button",{name:"通知品牌办理退出",exact:true})).toHaveCount(0);
    for(const locale of ["zh","en","ja","ru"]){
      await page.getByTestId("language-selector").selectOption(locale);
      await page.screenshot({path:info.outputPath(`channel-completed-${locale}.png`),fullPage:true});
      expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
    }
    await page.getByTestId("language-selector").selectOption("zh");
    for(const resource of ["plans","procurement","marketing","pricing","gifts","business-status","alerts","commissions","customers","ledger","invoices","reports","payments"]){
      const response=page.waitForResponse(r=>r.url().includes(`/api/v1/operators/${f.channel}/${resource}`)&&r.request().method()==="GET");
      await page.goto(`/manage/${f.channel}/${resource}`);
      expect((await response).status(),resource).toBe(200);
      await expect(page.getByText("已由星河品牌接管，渠道经营已结束",{exact:true})).toBeVisible();
      await expect(page.getByRole("heading",{level:1}).first()).toBeVisible();
      await expect(page.getByRole("button",{name:/^(新增套餐|建立套餐|发布套餐|申请采购|创建采购订单|通知品牌办理退出|保存预警设置|开启 新客户注册|开启 套餐新购)$/})).toHaveCount(0);
      if(resource==="plans"){await expect(page.getByText("在售",{exact:true})).toHaveCount(0);await expect(page.getByText("可由客户授权续费",{exact:true})).toHaveCount(0);await expect(page.getByText("已停止新购",{exact:true})).toHaveCount(2)}
      if(["plans","business-status","invoices"].includes(resource))await page.screenshot({path:info.outputPath(`channel-${resource}.png`),fullPage:true});
    }
    const stale=await page.evaluate(async(channel)=>{const me=await(await fetch("/api/v1/auth/me")).json();const r=await fetch(`/api/v1/operators/${channel}/exits`,{method:"POST",headers:{"Content-Type":"application/json","Idempotency-Key":crypto.randomUUID(),"X-CSRF-Token":me.csrf_token},body:"{}"});return {status:r.status,body:await r.json()};},f.channel);
    expect(stale.status).toBe(409);expect(stale.body.error.code).toBe("channel_takeover_restricted");
    await page.setViewportSize({width:390,height:844});
    await page.goto(`/manage/${f.channel}/takeovers`);
    await expect(page.getByRole("heading",{name:"停业与接管",exact:true})).toBeVisible();
    await expect(page.getByText("接管完成",{exact:true})).toBeVisible();
    await page.screenshot({path:info.outputPath("channel-completed-mobile.png"),fullPage:true});
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
    await login(customer,f.customer_email,"customer-password-test",false);
    await customer.goto("/workspace/plans");
    await expect(customer.getByText("渠道月套餐",{exact:true})).toBeVisible();
    await expect(customer.getByText("渠道不限时套餐",{exact:true})).toBeVisible();
    await customer.screenshot({path:info.outputPath("customer-retained-rights.png"),fullPage:true});
    const plans=await customer.evaluate(async()=>await(await fetch("/api/v1/me/plans")).json());
    expect(plans.items).toHaveLength(2);
    for(const p of plans.items){expect(p.status).toBe("active");expect(p.auto_renew).toBe(false);expect(p.auto_renew_available).toBe(false)}
    expect(plans.items.some((p:any)=>p.expires_at===null)).toBe(true);
    const me=await customer.evaluate(async()=>await(await fetch("/api/v1/auth/me")).json());expect(me.operator.id).toBe(f.oem);
    await info.attach("actual-transferred-rights",{body:JSON.stringify(plans,null,2),contentType:"application/json"});
  }catch(e){await page.screenshot({path:info.outputPath("failure-staff.png"),fullPage:true});await info.attach("failure-body",{body:await page.locator("body").innerText(),contentType:"text/plain"});throw e}
  finally{await staffContext.close();await customerContext.close()}
});
