import { expect, it } from "vitest";
import { configurationSchema, prepareConfiguration, validateModelConfiguration, validatePriceConfiguration } from "./operation-config";
import type { JSONValue, Operation } from "@/lib/contracts";

const operation: Operation = { name: "price", label: "发布价格", method: "POST", path: "/api/v1/operators/operator-one/pricing", fields: [] };
it("omits fields belonging to the other pricing mode without losing precise follow multipliers", () => {
  const schema = configurationSchema(operation)!;
  const value: JSONValue = { action: "price", price: { id: "follow", model_id: "model-one", scope: { kind: "retail", seller_id: "operator-one", buyer_id: "stale-buyer" }, layer: "regular", effective_at: "2026-12-01T00:00:00Z", rule: { mode: "follow", components: [{ key: "stale" }], follow: { kind: "supply", seller_id: "parent", buyer_id: "operator-one" }, multiplier: "1.000000000000000000000001", model_multipliers: { "model-one": "1.123456789012345678901234" }, extension: "preserved" }, published: true } };
  const prepared = prepareConfiguration(value, schema) as { price: { scope: object; rule: object } };
  expect(prepared.price.scope).toEqual({ kind: "retail", seller_id: "operator-one" });
  expect(prepared.price.rule).toEqual({ mode: "follow", follow: { kind: "supply", seller_id: "parent", buyer_id: "operator-one" }, multiplier: "1.000000000000000000000001", model_multipliers: { "model-one": "1.123456789012345678901234" } });
});
it("checks tier continuity using exact 24 digit decimals", () => {
  const value: JSONValue = { price: { layer: "regular", rule: { mode: "manual", components: [{ key: "quantity", per: "1", tiers: [{ from: "0", to: "0.000000000000000000000001", rate: "1" }, { from: "0.000000000000000000000002", rate: "1" }] }] } } };
  expect(() => validatePriceConfiguration(value)).toThrow("连续衔接");
});
it("requires a finite promotion end time and an unbounded final tier", () => {
  expect(() => validatePriceConfiguration({ price: { layer: "promotion" } })).toThrow("结束时间");
  expect(() => validatePriceConfiguration({ price: { layer: "regular", rule: { mode: "manual", components: [{ key: "quantity", per: "1", tiers: [{ from: "0", to: "100", rate: "1" }] }] } } })).toThrow("最后一阶");
});

const catalogOperation: Operation = { name:"save",label:"添加公开模型",method:"POST",path:"/api/v1/operators/operator-one/catalog",fields:[] };
const supplierOperation:Operation={name:"model",label:"添加供应商模型",method:"POST",path:"/api/v1/operators/operator-one/providers",fields:[]};
it("supplier setup needs protocol support and an optional reference, no price formula",()=>{
 const prepared=prepareConfiguration({supplier_model:{id:"s",credential_id:"key",provider:"openai",model:"vendor-model",enabled:true,capabilities:[{operation:"chat",streaming:true}],pricing_reference:"openai|gpt-test|chat"},expected_version:0},configurationSchema(supplierOperation)!);
 expect(()=>validateModelConfiguration(prepared)).not.toThrow();
 const model=(prepared as {supplier_model:Record<string,JSONValue>}).supplier_model;
 expect(model.capabilities).toEqual([{operation:"chat",streaming:true}]);
 expect(model).not.toHaveProperty("prices");expect(model).not.toHaveProperty("endpoints");
});
it("keeps a public model to its name and checked capabilities", () => {
  const prepared = prepareConfiguration({ model: { id: "public", enabled: false, route_configured: true, capabilities: [{ operation: "chat", usage_rules: [{ component: "input" }], evidence_id: "old" }] }, expected_version: 0, provider_extension: { exact: "1" } }, configurationSchema(catalogOperation)!);
  expect(() => validateModelConfiguration(prepared)).not.toThrow();
  expect(prepared).toEqual({ model: { id: "public", capabilities: [{ operation: "chat" }] }, expected_version: 0 });
  expect(prepared).toMatchObject({ model: { capabilities: [{ operation: "chat" }] } });
  const capability = (prepared as { model: { capabilities: Record<string, unknown>[] } }).model.capabilities[0];
  expect(capability.usage_rules).toBeUndefined();
  expect(capability.evidence_id).toBeUndefined();
});
it("requires complete nested pricing objects and preserves an explicit absent promotion end", () => {
  expect(() => prepareConfiguration({ action: "price" }, configurationSchema(operation)!)).toThrow("价目与适用范围");
  const value: JSONValue = { action: "bootstrap_price", price: { id: "new-price", model_id: "model-one", scope: { kind: "retail", seller_id: "operator-one" }, layer: "regular", effective_at: "2026-12-01T00:00:00Z", ends_at: null, rule: { mode: "manual", components: [{ key: "image", unit: "image", per: "1", rate: "0" }] }, published: true } };
  const prepared = prepareConfiguration(value, configurationSchema(operation)!);
  expect(() => validatePriceConfiguration(prepared)).not.toThrow();
  expect(prepared).toMatchObject({ price: { ends_at: null } });
});
it("enforces the published wildcard and model override rules", () => {
  expect(() => validatePriceConfiguration({ price: { model_id: "*", layer: "regular", rule: { mode: "manual" } } })).toThrow("默认规则");
  expect(() => validatePriceConfiguration({ price: { model_id: "*", layer: "regular", rule: { mode: "follow", multiplier: "1", model_multipliers: { "*": "2" } } } })).toThrow("实际模型");
});

it("supports megapixels independently and hides pricing identity fields",()=>{
 const template=configurationSchema({...operation,name:"template"})!;
 expect(template.fields?.billing.fields?.input.item?.fields?.template.options).toContainEqual(["megapixels","按百万像素"]);
 expect(template.fields?.billing.fields?.output.item?.fields?.modality.options).toContainEqual(["audio","音频"]);
 const price=configurationSchema(operation)!;
 const component=price.fields?.price.fields?.rule.fields?.components.item;
 expect(component?.presentation).toBe("price");expect(component?.fields?.key.readOnly).toBe(true);
});

it("rejects overlapping aggregate and modality input meters",()=>{
 expect(()=>validateModelConfiguration({action:"billing",billing:{input:[{modality:"all",template:"tokens"},{modality:"image",template:"images"}],output:[]}})).toThrow("不能在同一方向并用");
 expect(()=>validateModelConfiguration({action:"billing",billing:{input:[{modality:"text",template:"tokens"},{modality:"image",template:"megapixels"}],output:[{modality:"text",template:"tokens"}]}})).not.toThrow();
});
