/** @vitest-environment jsdom */
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { withZh } from "@/lib/test-i18n";
import Page from "./page";
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });
const order = { id: "pay_one", user_id: "usr_one", user_email: "alice@example.test", user_name: "Alice", channel_code: "official-a", adapter: "alipay", purpose: "wallet", status: "pending", currency: "CNY", amount_minor: 10000, credit_minor: 13986013, created_at: "2026-09-18T00:00:00Z" };
it("shows human payment and credit amounts, then confirms the selected recipient", async () => {
  const fetcher = vi.fn(async (_url: unknown, init?: RequestInit) => ({ ok: true, json: async () => init?.method === "POST" ? { item: { ...order, status: "paid", fulfilled_at: "2026-09-18" } } : { items: [order] } }));
  vi.stubGlobal("fetch",fetcher);render(withZh(<Page />));
  await screen.findByText("alice@example.test");
  expect(screen.getByText("¥100.00 CNY")).toBeTruthy();
  expect(screen.getByText(/充值额度 \$13.99 USD/)).toBeTruthy();
  expect(screen.queryByRole("button",{name:"退款",exact:true})).toBeNull();
  fireEvent.click(screen.getByRole("button",{name:"确认收款"}));
  const dialog=screen.getByRole("dialog");
  expect(within(dialog).getByText(/alice@example.test.*official-a.*pay_one.*¥100.00 CNY/)).toBeTruthy();
  fireEvent.click(within(dialog).getByRole("button",{name:"确认",exact:true}));
  expect(await screen.findByText(/订单 pay_one：已支付，额度已到账/)).toBeTruthy();
  expect(fetcher.mock.calls.some(([url,init]) => String(url).endsWith("/pay_one/confirm") && init?.method === "POST")).toBe(true);
});
it("keeps a refund error inside its confirmation and leaves the row unchanged", async () => {
  vi.stubGlobal("fetch",vi.fn(async (_url,init) => ({ok:init?.method !== "POST",json:async()=>init?.method === "POST"?{error:{message:"余额不足，退款未完成"}}:{items:[{...order,status:"paid",fulfilled_at:"2026-09-18"}]}})));
  render(withZh(<Page />));fireEvent.click(await screen.findByRole("button",{name:"退款",exact:true}));
  fireEvent.click(within(screen.getByRole("dialog")).getByRole("button",{name:"确认",exact:true}));
  expect(await within(screen.getByRole("dialog")).findByText("余额不足，退款未完成")).toBeTruthy();
});
it("successful action stays successful if the following list refresh fails", async()=>{
 let loads=0;
 vi.stubGlobal("fetch",vi.fn(async(_url,init)=>({ok:init?.method==="POST"||++loads===1,json:async()=>init?.method==="POST"?{item:{...order,status:"paid",fulfilled_at:"2026-09-18"}}:loads===1?{items:[order]}:{error:{message:"列表暂不可用"}}})));
 render(withZh(<Page/>));fireEvent.click(await screen.findByRole("button",{name:"确认收款"}));fireEvent.click(screen.getByRole("button",{name:"确认",exact:true}));
 await screen.findByText(/已支付，额度已到账/);await screen.findByText("列表暂不可用");expect(screen.queryByRole("dialog")).toBeNull();
});
it("loading failure is actionable and does not masquerade as an empty account",async()=>{
 vi.stubGlobal("fetch",vi.fn().mockRejectedValueOnce(new Error()).mockResolvedValueOnce({ok:true,json:async()=>({items:[]})}));render(withZh(<Page/>));
 await screen.findByText("连接失败，请重试加载订单。");expect(screen.queryByText(/没有符合条件/)).toBeNull();fireEvent.click(screen.getByRole("button",{name:"重试",exact:true}));await screen.findByText(/没有符合条件的订单/);
});
