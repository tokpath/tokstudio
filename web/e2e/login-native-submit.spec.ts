import { expect,test } from "@playwright/test";

test("unhydrated login cannot put credentials into a native GET URL",async({browser,baseURL})=>{
 const context=await browser.newContext({javaScriptEnabled:false});const page=await context.newPage();
 await page.goto(`${baseURL}/login?next=%2Fapp%2Fkeys`);
 const form=page.locator("form");
 await expect(form).toHaveAttribute("method","post");
 await expect(page.getByLabel("邮箱",{exact:true})).toBeDisabled();
 await expect(page.getByLabel("密码",{exact:true})).toBeDisabled();
 await expect(page.getByRole("button",{name:"登录",exact:true})).toBeDisabled();
 // Even a native/programmatic submit bypassing the disabled button is POST.
 let method="";
 await page.route("**/login?next=*",route=>{
  method=route.request().method();
  return route.fulfill({status:405,contentType:"text/html",body:"<p>JavaScript is required to sign in.</p>"});
 });
 await page.evaluate(()=>{
  const form=document.querySelector("form")!;
  (form.querySelector("input[name=email]") as HTMLInputElement).value="synthetic@example.test";
  (form.querySelector("input[name=password]") as HTMLInputElement).value="synthetic-password";
  form.submit();
 });
 await expect.poll(()=>method).toBe("POST");
 await page.waitForLoadState("domcontentloaded");
 const url=new URL(page.url());expect(url.searchParams.has("email")).toBe(false);expect(url.searchParams.has("password")).toBe(false);
 await context.close();
});
