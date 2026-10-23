import { describe, expect, it } from "vitest";
import { calculatePurchase, purchaseTiers } from "./purchase-tiers";
const tiers = [{from_usd:"0",gift_percent:"0"},{from_usd:"1000",gift_percent:"1"},{from_usd:"2000",gift_percent:"2"}];
describe("procurement tier preview",()=>{
 it("applies marginal percentages, rather than applying the highest tier to the entire order",()=>{
  expect(calculatePurchase(tiers,"1000")).toEqual({paid:"1000",gift:"0",total:"1000"});
  expect(calculatePurchase(tiers,"2000")).toEqual({paid:"2000",gift:"10",total:"2010"});
  expect(calculatePurchase(tiers,"2500")).toEqual({paid:"2500",gift:"20",total:"2520"});
  expect(calculatePurchase(tiers,"1000.01").gift).toBe("0.0001");
 });
 it("rejects malformed thresholds and keeps exact fractional arithmetic",()=>{
  expect(()=>purchaseTiers([{from_usd:"100",gift_percent:"1"}])).toThrow();
  expect(()=>purchaseTiers([...tiers,{from_usd:"1000",gift_percent:"3"}])).toThrow();
  expect(calculatePurchase([{from_usd:"0",gift_percent:"1"}],"0.0000000000000001").gift).toBe("0.000000000000000001");
  expect(()=>calculatePurchase(tiers,"0")).toThrow();
 });
});
