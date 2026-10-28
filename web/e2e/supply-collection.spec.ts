import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";
import { translate } from "../lib/i18n";

const ready = process.env.TOKENHUB_STOCK_ACCEPTANCE_FILE;
test.skip(!ready, "Requires the isolated real stock purchase application and PostgreSQL");

test("supplier records a cash payment without bank accounts, evidence or an external reference", async ({ page }, info) => {
  const fixture = JSON.parse(readFileSync(ready!, "utf8"));
  await page.goto("/login");
  await page.getByLabel("邮箱", { exact: true }).fill(fixture.owner_email);
  await page.getByLabel("密码", { exact: true }).fill(fixture.owner_password);
  await page.getByRole("button", { name: "登录", exact: true }).click();
  await page.waitForURL(/\/manage\//);
  await page.goto(`/manage/${fixture.operator_id}/procurement?section=quotes`);
  await expect(page.getByRole("link", { name: "采购与供货", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "按金额采购", exact: true }).click();
  const purchase = page.getByRole("dialog").last();
  await purchase.getByLabel("本次采购USD", { exact: true }).fill("1000");
  await purchase.getByRole("checkbox").check();
  const created = page.waitForResponse(response => response.url().endsWith("/purchase") && response.request().method() === "POST");
  await purchase.getByRole("button", { name: "按金额采购", exact: true }).click();
  const response = await created;
  expect(response.status()).toBe(200);
  const order = await response.json();
  expect(order.status).toBe("awaiting_payment");
  await purchase.getByRole("button", { name: "关闭办理面板", exact: true }).click();

  const supplier = `/manage/${fixture.supplier_id}`;
  await page.goto(supplier + "/orders");
  const salesRow = page.getByRole("row").filter({ hasText: "OEM3-1 channel" });
  await expect(salesRow).toHaveCount(1);
  await expect(salesRow.getByText("供货采购", { exact: true })).toBeVisible();
  await salesRow.getByRole("link", { name: "查看并办理" }).click();
  await expect(page.getByRole("heading", { name: "供货采购订单", exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: "登记本单线下收款", exact: true })).toBeVisible();
  await page.screenshot({ path: info.outputPath("wholesale-order-in-finance.png"), fullPage: true });

  await page.getByRole("button", { name: "登记本单线下收款", exact: true }).click();
  const contextual = page.getByRole("dialog").last();
  const optionalLabels = ["收款流水或收据号（可选）", "收款方资料（可选）", "付款方资料（可选）", "收款说明或凭证（可选）"];
  for (const label of optionalLabels) await expect(contextual.getByLabel(label, { exact: true })).toHaveJSProperty("required", false);
  await contextual.getByLabel("收款总额（扣手续费前） USD", {exact:true}).fill("1000");
  await expect(contextual.getByText("手续费待核实，净到账金额暂不确定。",{exact:true})).toBeVisible();
  await contextual.getByLabel("收款手续费核实状态",{exact:true}).selectOption("confirmed");
  await contextual.getByLabel("己方承担的收款手续费 USD",{exact:true}).fill("10");
  await expect(contextual.getByRole("region",{name:"收款金额核对"}).getByLabel("990 USD",{exact:true})).toBeVisible();
  await contextual.getByRole("button",{name:"了解己方承担的收款手续费 USD",exact:true}).click();
  await expect(page.getByRole("dialog").last()).toContainText("付款人另付的转账费、链上手续费不填这里");
  await page.getByRole("dialog").last().getByRole("button",{name:"关闭",exact:true}).click();
  for (const locale of ["en","ja","ru","zh"] as const) {
    await contextual.getByTestId("language-selector").selectOption(locale);
    await expect(contextual.getByLabel(translate(locale,"己方承担的收款手续费 USD"),{exact:true})).toHaveValue("10");
    await expect(contextual.getByRole("region",{name:translate(locale,"收款金额核对")}).getByLabel("990 USD",{exact:true})).toBeVisible();
  }
  await page.screenshot({path:info.outputPath("contextual-receipt-fees-and-net.png"),fullPage:true});
  await page.setViewportSize({width:390,height:844});
  await contextual.getByRole("region",{name:"收款金额核对"}).scrollIntoViewIfNeeded();
  expect(await contextual.evaluate(el=>el.scrollWidth<=el.clientWidth)).toBe(true);
  await page.screenshot({path:info.outputPath("receipt-net-mobile.png"),fullPage:true});
  await page.setViewportSize({width:1365,height:960});
  await contextual.getByRole("button", { name: "关闭办理面板", exact: true }).click();

  await page.goto(supplier + "/payments");
  await page.getByRole("button", { name: "登记人工核实收款", exact: true }).click();
  const receipt = page.getByRole("dialog").last();
  await receipt.getByRole("combobox", { name: "关联订单（可选）" }).fill("OEM3-1");
  await page.getByRole("option", { name: /供货采购.*OEM3-1 channel.*1000 USD/ }).click();
  for (const label of optionalLabels) await expect(receipt.getByLabel(label, { exact: true })).toHaveJSProperty("required", false);
  await receipt.getByLabel("收款总额（扣手续费前） USD", { exact: true }).fill("1000");
  await receipt.getByLabel("收款手续费核实状态",{exact:true}).selectOption("confirmed");
  await receipt.getByLabel("己方承担的收款手续费 USD",{exact:true}).fill("10");
  await expect(receipt.getByRole("region",{name:"收款金额核对"}).getByLabel("990 USD",{exact:true})).toBeVisible();
  await receipt.getByLabel("真实付款时刻", { exact: true }).fill(new Date().toISOString().slice(0, 16));
  await page.screenshot({ path: info.outputPath("wholesale-receipt-order-selected.png"), fullPage: true });
  const collected = page.waitForResponse(response => response.url().endsWith("/payments") && response.request().method() === "POST");
  await receipt.getByRole("button", { name: "登记人工核实收款", exact: true }).click();
  const collectedResponse = await collected;
  expect(collectedResponse.status()).toBe(200);
  const payload = collectedResponse.request().postDataJSON();
  const body = payload.configuration || payload;
  expect(body).toMatchObject({gross_usd:"1000",fee_usd:"10",fee_status:"confirmed"});
  expect(body.net_usd).toBeUndefined();
  for (const field of ["provider_receipt_id", "recipient_reference", "payer_reference", "verification_evidence"]) expect(body[field] || "").toBe("");
  const receiptResult = await collectedResponse.json();
  expect(receiptResult.message || "").not.toContain("尚未关联");
  await receipt.getByRole("button", { name: "关闭办理面板", exact: true }).click();

  await page.goto(supplier + `/orders?record=${order.id}`);
  await expect(page.getByText("采购额度已交付，下方可核对原收款。", { exact: true })).toBeVisible();
  await expect(page.getByRole("tabpanel", { name: "关联收款" }).getByRole("row")).toHaveCount(2);
  await expect(page.getByRole("button", { name: "登记本单线下收款", exact: true })).toHaveCount(0);
  await expect(page.getByRole("tabpanel",{name:"关联收款"}).getByLabel("990 USD",{exact:true})).toBeVisible();
  await page.screenshot({ path: info.outputPath("wholesale-payment-and-delivery-complete.png"), fullPage: true });
});
