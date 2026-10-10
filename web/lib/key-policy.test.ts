import {describe,it,expect} from "vitest";
import {usdToMinor,keyAllowsModel,keyState,preferredKeyModel} from "./key-policy";

describe("logical Key limits",()=>{
 it("preserves exact USD micro units and rejects invalid or unsafe limits",()=>{
  expect(usdToMinor("0.000001")).toBe(1);expect(usdToMinor("12.345678")).toBe(12_345_678);
  for(const invalid of ["0","-1","1e2","0.0000001","9007199255","abc"]){expect(usdToMinor(invalid)).toBeNull();}
 });
 it("does not widen an empty explicit selection to all models",()=>{
  expect(keyAllowsModel({model_mode:"selected",allowlist:[]},"m")).toBe(false);
  expect(keyAllowsModel({model_mode:"all",allowlist:[]},"m")).toBe(true);
 });
 it("distinguishes settled exhaustion from in-flight occupancy",()=>{
  expect(keyState({budget_limit_minor:100,budget_used_minor:80,budget_reserved_minor:20})).toBe("occupied");
  expect(keyState({budget_limit_minor:100,budget_used_minor:100})).toBe("spent");
  expect(keyState({status:"disabled",budget_limit_minor:100,budget_used_minor:100})).toBe("disabled");
 });
});

it("keeps a legal context or the unique allowed model in Key instructions",()=>{expect(preferredKeyModel({model_mode:"selected",allowlist:["chosen"]})).toBe("chosen");expect(preferredKeyModel({model_mode:"selected",allowlist:["chosen"]},"foreign")).toBe("chosen");expect(preferredKeyModel({model_mode:"selected",allowlist:["one","two"]},"two")).toBe("two");expect(preferredKeyModel({model_mode:"selected",allowlist:["one","two"]})).toBe("");});
