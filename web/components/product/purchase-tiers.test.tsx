import { describe, expect, it } from "vitest";
import { calculatePurchase, purchaseTiers } from "./purchase-tiers";
const tiers = [{from_money:"0",gift_percent:"0"},{from_money:"1000",gift_percent:"1"},{from_money:"2000",gift_percent:"2"}];
describe("procurement tier preview",()=>{
 it("applies marginal percentages, rather than applying the highest tier to the entire order",()=>{
  expect(calculatePurchase(tiers,"1000")).toEqual({paid:"1000",gift:"0",total:"1000"});
  expect(calculatePurchase(tiers,"2000")).toEqual({paid:"2000",gift:"10",total:"2010"});
  expect(calculatePurchase(tiers,"2500")).toEqual({paid:"2500",gift:"20",total:"2520"});
  expect(calculatePurchase(tiers,"1000.01").gift).toBe("0.0001");
 });
 it("rejects malformed thresholds and keeps exact fractional arithmetic",()=>{
  expect(()=>purchaseTiers([{from_money:"100",gift_percent:"1"}])).toThrow();
  expect(()=>purchaseTiers([...tiers,{from_money:"1000",gift_percent:"3"}])).toThrow();
  expect(calculatePurchase([{from_money:"0",gift_percent:"1"}],"0.0000000000000001").gift).toBe("0.000000000000000001");
  expect(()=>calculatePurchase(tiers,"0")).toThrow();
 });
 it("converts paid inventory and marginal gifts with the same configured rate",()=>{
  expect(calculatePurchase(tiers,"2500","2.5")).toEqual({paid:"6250",gift:"50",total:"6300"});
  expect(calculatePurchase([{from_money:"0",gift_percent:"1"}],"0.0000000000000001","10")).toEqual({paid:"0.000000000000001",gift:"0.00000000000000001",total:"0.00000000000000101"});
  expect(()=>calculatePurchase(tiers,"100","0")).toThrow();
  expect(()=>calculatePurchase(tiers,"0.000000000000000001","0.1")).toThrow();
 });
});
