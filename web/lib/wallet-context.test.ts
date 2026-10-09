import {describe,expect,it} from "vitest";
import {walletReturnPath,walletTabHref,purchaseIsResolved} from "./wallet-context";
describe("wallet task context",()=>{
 it("retains an already paid operation until actual fulfillment",()=>{
  expect(purchaseIsResolved({status:"paid"})).toBe(false);
  expect(purchaseIsResolved({status:"paid",fulfilled_at:"2026-10-10"})).toBe(true);
  expect(purchaseIsResolved({status:"failed"})).toBe(true);
  expect(purchaseIsResolved({status:"unknown"})).toBe(false);
 });
 it("preserves an actual task and selected plan across tabs",()=>{
  const url=new URL(walletTabHref("plan=monthly&next=%2Fapp%2Fkeys%3Fmodel%3Decho&ledger_cursor=old","plans"),"https://brand.example");
  expect(url.pathname).toBe("/app/wallet");expect(url.searchParams.get("next")).toBe("/app/keys?model=echo");expect(url.searchParams.get("plan")).toBe("monthly");
 });
 it("does not make a wallet or legacy plan return loop or open redirect",()=>{
  for(const raw of ["/app/wallet?tab=plans","/app/plans","/console/wallet","https://elsewhere.test","//elsewhere.test"]){expect(walletReturnPath(raw)).toBe("");}
  expect(walletReturnPath("/models/echo?task=key")).toBe("/models/echo?task=key");
 });
});
