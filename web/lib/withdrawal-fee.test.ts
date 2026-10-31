import { expect, it } from "vitest";
import { previewWithdrawal } from "./withdrawal-fee";

it("cuts a withdrawal to the currency unit and rounds the fee half up", () => {
  expect(previewWithdrawal({ amount: "10.0134", flatFee: "0", feeRate: "0.02", minimum: "1", digits: 2 })).toEqual({ ok: true, amount: "10.01", fee: "0.2", net: "9.81" });
  expect(previewWithdrawal({ amount: "10", flatFee: "1", feeRate: "0", minimum: "1", digits: 2 })).toEqual({ ok: true, amount: "10", fee: "1", net: "9" });
  expect(previewWithdrawal({ amount: "0.009", flatFee: "0", feeRate: "0", minimum: "0.001", digits: 2 }).ok).toBe(false);
  expect(previewWithdrawal({ amount: "0.5", flatFee: "0", feeRate: "0", minimum: "1", digits: 2 })).toMatchObject({ ok: false, reason: "below_minimum" });
});
