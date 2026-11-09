import { expect, it } from "vitest";
import { previewWithdrawal, previewPayout } from "./withdrawal-fee";

it("cuts a withdrawal to the currency unit and rounds the fee half up", () => {
  expect(previewWithdrawal({ amount: "10.0134", flatFee: "0", feeRate: "0.02", minimum: "1", digits: 2 })).toEqual({ ok: true, amount: "10.01", fee: "0.2", net: "9.81" });
  expect(previewWithdrawal({ amount: "10", flatFee: "1", feeRate: "0", minimum: "1", digits: 2 })).toEqual({ ok: true, amount: "10", fee: "1", net: "9" });
  expect(previewWithdrawal({ amount: "0.009", flatFee: "0", feeRate: "0", minimum: "0.001", digits: 2 }).ok).toBe(false);
  expect(previewWithdrawal({ amount: "0.5", flatFee: "0", feeRate: "0", minimum: "1", digits: 2 })).toMatchObject({ ok: false, reason: "below_minimum" });
});

it("shows the actual receiving currency amount without floating-point rounding",()=>{
 expect(previewPayout("9.81","7.123",2)).toBe("69.88");
 expect(previewPayout("9.81","150.5",0)).toBe("1476");
 expect(previewPayout("9.81","1.001",6)).toBe("9.81981");
 expect(previewPayout("0.001","1",2)).toBeUndefined();
 expect(previewPayout("100","0",2)).toBeUndefined();
});
