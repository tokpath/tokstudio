// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { I18nProvider } from "@/components/i18n";
import { locales, translate } from "@/lib/i18n";
import type { Entity, ResourcePage } from "@/lib/contracts";
import { LedgerCollection } from "./ledger-workspace";

afterEach(cleanup);
const owner="Lin · lin@example.test";
const source={kind:"套餐购买",name:"Pro plan",amount:"100",unit:"USD",at:"2026-11-04T10:00:00Z",url:"/manage/operator/orders?record=order-uuid"};
const account:Entity={id:"account-uuid",values:{owner_id:"customer-uuid",purpose:"customer:atg:grant-uuid",balance:"100.000000000000000001",held:"1",unit:"ATG",display:{owner,purpose:"客户 ATG 权益",auxiliary:false,available:"99.000000000000000001",source,expires_at:"2026-12-04T10:00:00Z"}}};
const journal:Entity={id:"journal-uuid",values:{kind:"usage",reason:"confirmed model usage",created_at:"2026-11-05T10:00:00Z",business_key:"private-operation-key",entries:[{account_id:"account-uuid",owner_id:"customer-uuid",purpose:"customer:atg:grant-uuid",unit:"ATG",amount:"-0.000000000000000001",balance_after:"100.000000000000000001",display:account.values.display}]}};
const page=(items:Entity[]):ResourcePage=>({title:"资产与账本",description:"",items,columns:[],operations:[],row_operations:[]});
for(const locale of locales) {
 it(`shows readable ledger entries and preserves precision in ${locale}`,()=>{
  render(<I18nProvider initialLocale={locale}><LedgerCollection page={page([journal])} view="journals" /></I18nProvider>);
  expect(screen.getByText(owner)).toBeTruthy();expect(screen.getByText(translate(locale,"模型调用扣费"))).toBeTruthy();
  expect(screen.queryByText("journal-uuid")).toBeNull();expect(screen.queryByText("balance_after")).toBeNull();
  fireEvent.click(screen.getByRole("button",{name:translate(locale,"查看详情")}));
  const dialog=screen.getByRole("dialog");expect(within(dialog).getByRole("columnheader",{name:translate(locale,"本次入账后余额")})).toBeTruthy();
  expect(within(dialog).getByLabelText("-0.000000000000000001 ATG")).toBeTruthy();expect(within(dialog).getByLabelText("100.000000000000000001 ATG")).toBeTruthy();
  expect(within(dialog).getByRole("link",{name:translate(locale,"查看原业务")}).getAttribute("href")).toBe(source.url);
  const details=within(dialog).getByText(translate(locale,"原始记录与编号")).closest("details")!;
  expect(details.open).toBe(false);fireEvent.click(details.querySelector("summary")!);expect(within(dialog).getByText("private-operation-key")).toBeTruthy();
 });
 it(`names asset source and availability in ${locale}`,()=>{
  render(<I18nProvider initialLocale={locale}><LedgerCollection page={page([account])} view="accounts" /></I18nProvider>);
  expect(screen.getByText(owner)).toBeTruthy();expect(screen.getByText(/Pro plan/)).toBeTruthy();expect(screen.getByLabelText("99.000000000000000001 ATG")).toBeTruthy();
  expect(screen.queryByText("open")).toBeNull();expect(screen.queryByText("customer-uuid")).toBeNull();
 });
}
