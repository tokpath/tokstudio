import { expect, it } from "vitest";
import { configurationSchema, prepareConfiguration, validateModelConfiguration, validatePriceConfiguration } from "./operation-config";
import type { JSONValue, Operation } from "@/lib/contracts";

const operation: Operation = { name: "price", label: "发布价格", method: "POST", path: "/api/v1/operators/operator-one/pricing", fields: [] };
it("omits fields belonging to the other pricing mode without losing precise follow multipliers", () => {
  const schema = configurationSchema(operation)!;
  const value: JSONValue = { action: "price", price: { id: "follow", model_id: "model-one", scope: { kind: "retail", seller_id: "operator-one", buyer_id: "stale-buyer" }, layer: "regular", effective_at: "2026-12-01T00:00:00Z", rule: { mode: "follow", components: [{ key: "stale" }], follow: { kind: "supply", seller_id: "parent", buyer_id: "operator-one" }, multiplier: "1.000000000000000000000001", model_multipliers: { "model-one": "1.123456789012345678901234" }, extension: "preserved" }, published: true } };
  const prepared = prepareConfiguration(value, schema) as { price: { scope: object; rule: object } };
  expect(prepared.price.scope).toEqual({ kind: "retail", seller_id: "operator-one" });
  expect(prepared.price.rule).toEqual({ mode: "follow", follow: { kind: "supply", seller_id: "parent", buyer_id: "operator-one" }, multiplier: "1.000000000000000000000001", model_multipliers: { "model-one": "1.123456789012345678901234" }, extension: "preserved" });
});
it("checks tier continuity using exact 24 digit decimals", () => {
  const value: JSONValue = { price: { layer: "regular", rule: { mode: "manual", components: [{ key: "quantity", per: "1", tiers: [{ from: "0", to: "0.000000000000000000000001", rate: "1" }, { from: "0.000000000000000000000002", rate: "1" }] }] } } };
  expect(() => validatePriceConfiguration(value)).toThrow("连续衔接");
});
it("requires a finite promotion end time and an unbounded final tier", () => {
  expect(() => validatePriceConfiguration({ price: { layer: "promotion" } })).toThrow("结束时间");
  expect(() => validatePriceConfiguration({ price: { layer: "regular", rule: { mode: "manual", components: [{ key: "quantity", per: "1", tiers: [{ from: "0", to: "100", rate: "1" }] }] } } })).toThrow("最后一阶");
});

function verifiedModel(): JSONValue {
  return { model: { id: "verified-model", display_name: "业务模型", identity: "revision-one", enabled: false, capabilities: [{ operation: "image_generation", streaming: false, input_types: ["text"], parameters: ["temperature"], max_input_bytes: 1024, billable_bounds: { image: "1/3" }, usage_rules: [{ component: "image", unit: "image", path: "/images" }], parameter_constraints: { temperature: { min: "-1/3", max: "0.333333333333333333333333333333" } }, evidence_id: "probe-real", verified_at: "2026-10-18T00:00:00Z", parameters_verified: true, usage_verified: true, bounds_verified: true, recovery_verified: true }] }, expected_version: 3 };
}
const catalogOperation: Operation = { name: "save", label: "维护公开模型", method: "POST", path: "/api/v1/operators/operator-one/catalog", fields: [] };
it("validates fractional capability quantities and signed parameter limits without applying price decimal limits", () => {
  const value = prepareConfiguration(verifiedModel(), configurationSchema(catalogOperation)!);
  expect(() => validateModelConfiguration(value)).not.toThrow();
  expect(value).toMatchObject({ model: { capabilities: [{ billable_bounds: { image: "1/3" }, parameter_constraints: { temperature: { min: "-1/3", max: "0.333333333333333333333333333333" } } }] } });
});
it.each(["-1/2", "1/0", "not-a-number"])("rejects an invalid nonnegative capability bound %s", bound => {
  const value = verifiedModel() as { model: { capabilities: { billable_bounds: Record<string, string> }[] } };
  value.model.capabilities[0].billable_bounds.image = bound;
  expect(() => prepareConfiguration(value, configurationSchema(catalogOperation)!)).toThrow("有效分数");
});
it("requires complete nested pricing objects and preserves an explicit absent promotion end", () => {
  expect(() => prepareConfiguration({ action: "price" }, configurationSchema(operation)!)).toThrow("价目与适用范围");
  const value: JSONValue = { action: "bootstrap_price", price: { id: "new-price", model_id: "model-one", scope: { kind: "retail", seller_id: "operator-one" }, layer: "regular", effective_at: "2026-12-01T00:00:00Z", ends_at: null, rule: { mode: "manual", components: [{ key: "image", unit: "image", per: "1", rate: "0" }] }, published: true } };
  const prepared = prepareConfiguration(value, configurationSchema(operation)!);
  expect(() => validatePriceConfiguration(prepared)).not.toThrow();
  expect(prepared).toMatchObject({ price: { ends_at: null } });
});
it("rejects reversed exact rational parameter ranges and incomplete evidence", () => {
  const value = verifiedModel() as { model: { capabilities: { parameter_constraints: Record<string, { min: string; max: string }>; usage_verified: boolean }[] } };
  value.model.capabilities[0].parameter_constraints.temperature = { min: "1/3", max: "0.333333333333333333333333333333" };
  expect(() => validateModelConfiguration(prepareConfiguration(value, configurationSchema(catalogOperation)!))).toThrow("最小值");
  value.model.capabilities[0].usage_verified = false;
  expect(() => validateModelConfiguration(prepareConfiguration(value, configurationSchema(catalogOperation)!))).toThrow("验证必须全部通过");
});
it("matches the backend rational radix and exponent rules instead of reinterpreting integer fractions as decimal", () => {
  const value = verifiedModel() as { model: { capabilities: { parameter_constraints: Record<string, { min: string; max: string }>; billable_bounds: Record<string, string> }[] } };
  const capability = value.model.capabilities[0];
  capability.billable_bounds.image = "0b1/010";
  capability.parameter_constraints.temperature = { min: "0x1.8p-1", max: "3/4" };
  const prepare = () => prepareConfiguration(value, configurationSchema(catalogOperation)!);
  expect(() => validateModelConfiguration(prepare())).not.toThrow();
  capability.parameter_constraints.temperature = { min: "1/010", max: "0.1" };
  expect(() => validateModelConfiguration(prepare())).toThrow("最小值");
  capability.parameter_constraints.temperature = { min: "1_0e-2", max: "1/10" };
  expect(() => validateModelConfiguration(prepare())).not.toThrow();
});
it("enforces the published wildcard and model override rules", () => {
  expect(() => validatePriceConfiguration({ price: { model_id: "*", layer: "regular", rule: { mode: "manual" } } })).toThrow("默认规则");
  expect(() => validatePriceConfiguration({ price: { model_id: "*", layer: "regular", rule: { mode: "follow", multiplier: "1", model_multipliers: { "*": "2" } } } })).toThrow("实际模型");
});
