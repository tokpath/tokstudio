import { Decimal, type JSONValue, type Operation } from "@/lib/contracts";
import { compareDecimal } from "@/lib/decimal";
import { translate, type Locale } from "@/lib/i18n";

export type ConfigNode = {
  label: string;
  kind: "text" | "textarea" | "decimal" | "rational" | "integer" | "boolean" | "datetime" | "select" | "object" | "array" | "record";
  help?: string;
  required?: boolean;
  default?: JSONValue;
  generated?: "uuid";
  fields?: Record<string, ConfigNode>;
  item?: ConfigNode;
  options?: [string, string][];
  when?: { field: string; values: string[] };
  lookup?: "models" | "credentials" | "buyers" | "suppliers" | "routes" | "supplier-models";
  lookupBy?: { field: string; values: Record<string, "buyers" | "routes"> };
  lookupWhen?: { field: string; values: string[] };
  wildcard?: boolean;
  signed?: boolean;
  min?: number;
  max?: number;
  nullable?: boolean;
  readOnly?: boolean;
  /** Capability cards explain call, billing, and verification before the rare limits. */
  presentation?: "capability" | "operations";
  /** Hide a field when a sibling has one of these values. Hidden fields are omitted on submit. */
  hideWhen?: { field: string; values: string[] };
};
const text = (label: string, required = true, help?: string): ConfigNode => ({ kind: "text", label, required, help });
const decimal = (label: string, required = true): ConfigNode => ({ kind: "decimal", label, required });
const integer = (label: string, value?: number): ConfigNode => ({ kind: "integer", label, required: value === undefined, default: value });
const boolean = (label: string, value = false): ConfigNode => ({ kind: "boolean", label, default: value });
const datetime = (label: string, required = false): ConfigNode => ({ kind: "datetime", label, required });
const select = (label: string, options: [string, string][], value?: string): ConfigNode => ({ kind: "select", label, options, required: true, default: value });
const object = (label: string, fields: Record<string, ConfigNode>, help?: string): ConfigNode => ({ kind: "object", label, fields, required: true, help });
const array = (label: string, item: ConfigNode, help?: string): ConfigNode => ({ kind: "array", label, item, default: [], help });
const record = (label: string, item: ConfigNode, help?: string): ConfigNode => ({ kind: "record", label, item, help });
const operations: [string, string][] = [["chat", "对话"], ["responses", "Responses"], ["embedding", "向量"], ["image_generation", "图片生成"], ["speech", "语音生成"], ["transcription", "语音转写"], ["video_generation", "视频生成"], ["video_retrieve", "视频任务查询"], ["video_download", "视频结果下载"]];
const strings = (label: string) => array(label, text("值"));
const capability: ConfigNode = {
  ...object("能力", {
    operation: select("调用能力", operations),
    streaming: { ...boolean("支持流式输出"), help: "客户要求一段一段返回时，选「是」。这不影响预估金额。" },
    billable_bounds: { ...record("各计费组件的有限用量上界", { ...text("用量上界", true, "使用精确数量；可填写经验证的分数，例如 1/2。"), kind: "rational" }), required: true },
    usage_rules: array("计量组件", object("计量规则", { component: text("组件标识"), unit: text("计量单位"), path: text("用量路径", true, "供应商标准化用量中的 RFC 6901 路径，例如 /input_tokens。"), zero_when_absent: boolean("缺少此用量时可确认是零") })),
    subtract_paths: record("需要扣除的重复子用量", strings("扣除路径")),
  }, "保存这项能力前，选择调用方式，并至少填写一项计费。"),
  presentation: "capability",
};
const scope = (label: string, operatorID?: string) => object(label, {
  kind: select("价目用途", [["retail", "客户零售价"], ["supply", "对下级供货价"]], "retail"),
  seller_id: { ...text("定价经营方或供应商（自动带入）"), default: operatorID, readOnly: true },
  buyer_id: { ...text("供货采购方或成本上游路由"), lookupBy: {field:"kind",values:{supply:"buyers",supplier:"routes"}}, when: { field: "kind", values: ["supply", "supplier"] } },
});
const tier = object("阶梯", { from: decimal("起始数量（含）"), to: decimal("结束数量（不含，最后一阶可留空）", false), rate: decimal("本阶梯单价") });
const component = object("计费组件", {
  key: text("组件标识", true, "与模型计量组件完全一致，例如 input_tokens 或 image:1024x1024:high。"),
  unit: text("计量单位", true, "例如 token、image、second、request。"), per: { ...decimal("每多少单位"), default: "1" }, rate: decimal("基础单价"),
  tiers: array("渐进阶梯价格（可选）", tier, "从 0 连续衔接，区间左闭右开；最后一阶结束数量留空表示不限上界。"),
});

export function configurationSchema(operation: Operation, operatorID?: string): ConfigNode | undefined {
  if (/\/catalog$/.test(operation.path)) return object("公开模型", {
    model: object("模型名称与能力", {
      id: text("公开模型标识"),
      display_name: text("客户看到的模型名称", true, "客户选择模型时看到的名字，例如「对话 Pro」。以后可以修改。"),
      enabled: { ...boolean("开放模型"), help: "选「是」后客户才能调用。每一项已勾选的能力都要已经在路由配置里绑上可用的供应商模型。" },
      capabilities: { ...array("可调用的能力", object("能力", { operation: select("调用能力", operations.filter(([key]) => key !== "video_retrieve" && key !== "video_download")) }), "勾选客户可以调用的能力。计费和供应商价格写在供应商模型上。"), presentation: "operations", options: operations.filter(([key]) => key !== "video_retrieve" && key !== "video_download") },
    }),
    expected_version: { ...integer("当前记录版本（自动带入）", 0), readOnly: true },
    effective_at: { ...datetime("计划生效时间（修订已开放模型时填写）"), hideWhen: { field: "expected_version", values: ["0", ""] } },
    evidence: { ...text("变更及通知依据", false), kind: "textarea", hideWhen: { field: "expected_version", values: ["0", ""] } },
    emergency: { ...boolean("紧急停用（须有真实事件依据）"), hideWhen: { field: "expected_version", values: ["0", ""] } },
  });
  if (/\/providers$/.test(operation.path) && operation.name === "model") return object("供应商模型", {
    supplier_model: object("供应商模型与成本价", {
      id: { ...text("供应商模型标识"), generated: "uuid", readOnly: true },
      credential_id: { ...text("供应商密钥"), lookup: "credentials" },
      provider_id: { ...text("供应商引用（选择密钥后自动带入）", false), readOnly: true },
      provider: text("协议名称", true, "例如 openai。这是转发时使用的协议，不是给客户看的名字。"),
      model: text("供应商模型名称", true, "供应商要求填写的模型名。客户请求里的名字写在公开模型上。"),
      enabled: { ...boolean("启用这个供应商模型"), default: true },
      endpoints: record("服务地址", text("HTTPS 服务地址"), "一般留空，系统按协议使用默认地址。视频查询和下载要填写带 {task_id} 的地址。"),
      capabilities: array("怎么计费", capability, "每一项是一种调用。计费名称要和下面的成本价一致。"),
      prices: array("成本价（USD）", object("成本价", { key: text("计费名称", true, "与上面的计费名称一致，例如 input。"), unit: text("计量单位"), per: { ...decimal("每多少单位"), default: "1" }, rate: decimal("单价（USD）") }), "这是平台付给供应商的价格，不是客户看到的零售价。"),
    }, "填写供应商那边的模型名称、怎么读取用量，以及 USD 成本价。"),
    expected_version: { ...integer("当前记录版本（自动带入）", 0), readOnly: true },
  });
  if (/\/routes$/.test(operation.path)) return object("路由配置", {
    route: object("公开模型的路由", {
      public_model_id: { ...text("公开模型"), lookup: "models" },
      strategy: select("选路策略", [["priority", "固定优先级"], ["lowest_latency", "已验证最低延迟"], ["lowest_cost", "已验证最低成本"]], "priority"),
      bindings: array("绑定的供应商模型", object("绑定", {
        id: { ...text("绑定标识"), generated: "uuid", readOnly: true },
        supplier_model_id: { ...text("供应商模型"), lookup: "supplier-models" },
        priority: { ...integer("选路优先级", 0), signed: true },
        enabled: { ...boolean("这条绑定接单"), default: true },
        routing: { ...object("熔断", { max_concurrent: { ...integer("最大同时在途（0 不额外限制）", 0), max: 100000 }, failure_threshold: { ...integer("连续失败阈值（0 使用默认 3）", 0), max: 100 }, unknown_usage_threshold: { ...integer("未知用量阈值（0 使用默认 2）", 0), max: 100 }, recovery_successes: { ...integer("恢复成功探测次数（0 使用默认 2）", 0), max: 20 }, cooldown_seconds: { ...integer("冷却秒数（0 使用默认 60）", 0), max: 86400 } }), required: false },
      }), "一个公开模型可以绑多个供应商模型。数字小的优先级先被考虑。"),
    }, "选择先用哪一条，并写下连续失败后暂时停用的数字。"),
    expected_version: { ...integer("当前策略版本（自动带入）", 0), readOnly: true },
  });
  if (/\/pricing$/.test(operation.path) && operation.name === "listing") return object("授权与上架", {
    action: select("办理事项", [["listing", "本经营方上架 / 下架"], ["authorization", "授权 / 撤销下级模型权限"]], "listing"),
    buyer_id: { ...text("被授权的直属下级"), lookup: "buyers", when: { field: "action", values: ["authorization"] } }, model_id: { ...text("公开模型"), lookup: "models" }, enabled: boolean("允许使用此模型", true),
    expected_version: { ...integer("当前上架 / 授权版本（自动带入）", 0), required: true, readOnly: true }, effective_at: datetime("计划生效时间"), evidence: { ...text("变更及通知依据", false), kind: "textarea" },
  });
  if (/\/pricing$/.test(operation.path)) return object("模型价格", {
    action: select("发布方式", [["price", "按通知规则发布价格"], ["bootstrap_price", "初始化价格（仅首次开通）"]], "price"),
    price: object("价目与适用范围", {
      id: { ...text("本次价格版本标识"), generated: "uuid", readOnly: true }, model_id: { ...text("公开模型（自动跟随可用 * 表示默认规则）"), lookup: "models", wildcard: true }, scope: scope("本次定价范围", operatorID),
      layer: select("价格类别", [["regular", "常规价格"], ["promotion", "限时促销价"]], "regular"), effective_at: datetime("价格生效时间", true), ends_at: { ...datetime("价格结束时间（促销必填）"), nullable: true },
      rule: object("计价方式", {
        mode: select("计价方式", [["manual", "逐组件手动定价"], ["follow", "自动跟随上级价目"]], "manual"),
        components: { ...array("组件价格", component), when: { field: "mode", values: ["manual"] } },
        follow: { ...scope("跟随的上级价目"), fields: {
          kind: { ...select("跟随价目用途", [["supply", "直接上级对本经营方的供货价"]], "supply"), readOnly: true },
          seller_id: { ...text("直接上级经营方"), lookup: "suppliers" }, buyer_id: { ...text("供货买方（本经营方）"), default: operatorID, readOnly: true },
        }, when: { field: "mode", values: ["follow"] } },
        multiplier: { ...decimal("默认倍率（定价参数）"), when: { field: "mode", values: ["follow"] } },
        model_multipliers: { ...record("指定模型倍率覆盖（可选）", decimal("模型倍率")), when: { field: "mode", values: ["follow"] } },
      }, "零售价使用客户 ATG，供货价使用采购 ATG。供应商的 USD 成本价写在供应商模型上。倍率不是利润率。"),
      notice_id: text("客户通知记录引用", false), published: { ...boolean("发布此价格版本", true), required: true, readOnly: true },
    }), evidence: { ...text("发布与通知依据", false), kind: "textarea" },
  });
}

export function configDefaults(node: ConfigNode): JSONValue | undefined {
  if (node.default !== undefined) return node.default;
  if (node.generated === "uuid") return crypto.randomUUID();
  if (node.kind === "object") return Object.fromEntries(Object.entries(node.fields || {}).flatMap(([key, child]) => { const value = configDefaults(child); return value === undefined ? [] : [[key, value]]; }));
  if (node.required) return "";
}

export function nodeVisible(node: ConfigNode, parent: Record<string, JSONValue>): boolean {
  if (node.hideWhen && node.hideWhen.values.includes(String(parent[node.hideWhen.field] ?? ""))) return false;
  return !node.when || node.when.values.includes(String(parent[node.when.field] ?? ""));
}

// Prune only fields that are explicitly conditional. All unknown extension
// fields survive a structured edit, and all monetary values stay as strings.
type ConfigurationPathPart = { source: string; separator?: string; literal?: boolean };

export class ConfigurationError extends Error {
  constructor(public source: string, public fieldPath = "", public values: Record<string, string | number> = {}, private pathParts?: ConfigurationPathPart[]) {
    super(translate("zh", source, values)); this.name = "ConfigurationError";
  }
  localized(locale: Locale): string {
    const values = Object.fromEntries(Object.entries(this.values).map(([key, value]) => [key,
      key === "path" && this.pathParts ? this.pathParts.map(part => `${part.separator || ""}${part.literal ? part.source : translate(locale, part.source)}`).join("")
        : typeof value === "string" ? translate(locale, value) : value]));
    return translate(locale, this.source, values);
  }
}

// Capability quantities and constraints are rational strings, unlike prices.
// Compare their integer numerator/denominator without ever rounding a quantity.
function rational(value: string): [bigint, bigint] | undefined {
  const radix = (raw: string, fraction: boolean) => {
    const prefix = /^0([bBoOxX])/.exec(raw)?.[1]?.toLowerCase();
    const base = prefix === "b" ? 2 : prefix === "o" ? 8 : prefix === "x" ? 16 : fraction && raw.length > 1 && raw[0] === "0" ? 8 : 10;
    return { base, digits: prefix ? raw.slice(2) : base === 8 && fraction && raw[0] === "0" ? raw.slice(1) : raw, prefixed: !!prefix || base === 8 && fraction };
  };
  const digitSet = (base: number) => base === 2 ? "[01]" : base === 8 ? "[0-7]" : base === 16 ? "[\\da-fA-F]" : "\\d";
  const digitsValid = (raw: string, base: number, prefix = false) => new RegExp(`^${prefix ? "_?" : ""}${digitSet(base)}(?:_?${digitSet(base)})*$`).test(raw);
  const integer = (raw: string, signed: boolean): bigint | undefined => {
    const sign = signed && /^[+-]/.test(raw) ? raw[0] : "", unsigned = sign ? raw.slice(1) : raw, parsed = radix(unsigned, true);
    if (!digitsValid(parsed.digits, parsed.base, parsed.prefixed)) return;
    const digits = parsed.digits.replaceAll("_", ""), prefix = parsed.base === 2 ? "0b" : parsed.base === 8 ? "0o" : parsed.base === 16 ? "0x" : "";
    return BigInt(prefix + digits) * (sign === "-" ? -1n : 1n);
  };
  if (value.includes("/")) {
    const parts = value.split("/");
    if (parts.length !== 2) return;
    const numerator = integer(parts[0], true), denominator = integer(parts[1], false);
    return numerator !== undefined && denominator !== undefined && denominator > 0n ? [numerator, denominator] : undefined;
  }
  const sign = /^[+-]/.test(value) ? value[0] : "", unsigned = sign ? value.slice(1) : value, parsed = radix(unsigned, false);
  const exponentIndex = parsed.digits.search(parsed.base === 16 ? /[pP]/ : /[eEpP]/);
  const mantissa = exponentIndex < 0 ? parsed.digits : parsed.digits.slice(0, exponentIndex), exponentText = exponentIndex < 0 ? "" : parsed.digits.slice(exponentIndex);
  if (exponentText && !/^[eEpP][+-]?\d(?:_?\d)*$/.test(exponentText)) return;
  const exponent = exponentText ? BigInt(exponentText.slice(1).replaceAll("_", "")) : 0n;
  if (exponent < -(2n ** 63n) || exponent >= 2n ** 63n) return;
  const parts = mantissa.split(".");
  if (parts.length > 2 || !parts.some(part => part.replaceAll("_", ""))) return;
  if (parts.some((part, index) => part && !digitsValid(part, parsed.base, index === 0 && parsed.prefixed))) return;
  const digits = parts.map(part => part.replaceAll("_", "")).join(""), prefix = parsed.base === 2 ? "0b" : parsed.base === 8 ? "0o" : parsed.base === 16 ? "0x" : "";
  let numerator = BigInt(prefix + digits) * (sign === "-" ? -1n : 1n);
  if (numerator === 0n) return [0n, 1n];
  const places = BigInt((parts[1] || "").replaceAll("_", "").length), decimalExponent = !exponentText || /^[eE]/.test(exponentText);
  const exponent2 = exponent - places * BigInt(parsed.base === 16 ? 4 : parsed.base === 8 ? 3 : 1), exponent5 = (decimalExponent ? exponent : 0n) - (parsed.base === 10 ? places : 0n);
  if (exponent5 < -1000000n || exponent5 > 1000000n || exponent2 < -10000000n || exponent2 > 10000000n) return;
  let denominator = 1n;
  if (exponent2 >= 0n) numerator *= 2n ** exponent2; else denominator *= 2n ** -exponent2;
  if (exponent5 >= 0n) numerator *= 5n ** exponent5; else denominator *= 5n ** -exponent5;
  return [numerator, denominator];
}
export function prepareConfiguration(value: JSONValue, node: ConfigNode, path = node.label, fieldPath = "", pathParts: ConfigurationPathPart[] = [{ source: path }]): JSONValue {
  const fail = (message: string, values: Record<string, string | number> = { path }): never => { throw new ConfigurationError(message, fieldPath, values, pathParts); };
  if (value === null && node.nullable) return null;
  if (value === null && !node.required && ["object", "array", "record"].includes(node.kind)) return null;
  if (node.kind === "object") {
    if (!value || Array.isArray(value) || typeof value !== "object") return fail("{path}应为一组业务字段");
    const out = { ...value };
    for (const [key, child] of Object.entries(node.fields || {})) {
      if (!nodeVisible(child, value)) { delete out[key]; continue; }
      if (out[key] !== undefined && out[key] !== "") out[key] = prepareConfiguration(out[key], child, `${path} · ${child.label}`, fieldPath ? `${fieldPath}.${key}` : key, [...pathParts, { source: child.label, separator: " · " }]);
      else if (child.required) throw new ConfigurationError(child.readOnly ? "无法核实{label}，请重新选择业务记录后重试。" : "请填写{path} · {label}", child.readOnly && node.fields?.model_id?.lookup ? (fieldPath ? `${fieldPath}.model_id` : "model_id") : (fieldPath ? `${fieldPath}.${key}` : key), { path, label: child.label }, pathParts);
      else if (out[key] === "") delete out[key];
    }
    if (node.presentation === "capability") for (const key of ["input_types", "parameters", "required_parameters", "parameter_constraints", "output_limit_parameter", "max_output", "max_input_bytes", "evidence_id", "verified_at", "parameters_verified", "usage_verified", "bounds_verified", "recovery_verified"]) delete out[key];
    if (node.fields?.operation && !node.fields.usage_rules) for (const key of ["input_types", "parameters", "required_parameters", "parameter_constraints", "output_limit_parameter", "max_output", "max_input_bytes", "evidence_id", "verified_at", "parameters_verified", "usage_verified", "bounds_verified", "recovery_verified", "usage_rules", "billable_bounds", "subtract_paths", "streaming"]) delete out[key];
    return out;
  }
  if (node.kind === "array") {
    if (!Array.isArray(value)) return fail("{path}应为列表");
    return value.map((item, index) => prepareConfiguration(item, node.item!, `${path} ${index + 1}`, `${fieldPath}.${index}`, [...pathParts, { source: String(index + 1), separator: " ", literal: true }]));
  }
  if (node.kind === "record") {
    if (!value || Array.isArray(value) || typeof value !== "object") return fail("{path}应为按名称配置的字段");
    return Object.fromEntries(Object.entries(value).map(([key, item]) => {
      if (!key.trim()) return fail("请填写{path}的项目名称");
      return [key, prepareConfiguration(item, node.item!, `${path} · ${key}`, `${fieldPath}.${key}`, [...pathParts, { source: key, separator: " · ", literal: true }])];
    }));
  }
  if (node.kind === "decimal") { if (typeof value !== "string" || !Decimal.safeParse(value).success) return fail("{path}请输入非负十进制数，最多 24 位小数"); }
  else if (node.kind === "rational") { const parsed = typeof value === "string" ? rational(value) : undefined; if (!parsed || (!node.signed && parsed[0] < 0n)) return fail(node.signed ? "{path}请输入精确数量或有效分数，分母须大于 0" : "{path}请输入非负精确数量或有效分数，分母须大于 0"); }
  else if (node.kind === "integer") {
    if (typeof value !== "number" || !Number.isSafeInteger(value) || (!node.signed && value < 0)) return fail(node.signed ? "{path}应为安全整数" : "{path}应为非负整数");
    if (node.min !== undefined && value < node.min) return fail("{path}不能小于 {min}", { path, min: node.min });
    if (node.max !== undefined && value > node.max) return fail("{path}不能大于 {max}", { path, max: node.max });
  }
  else if (node.kind === "boolean") { if (typeof value !== "boolean") return fail("{path}应选择是否启用"); }
  else if (node.kind === "datetime") { if (typeof value !== "string" || Number.isNaN(new Date(value).getTime())) return fail("{path}时间无效"); return new Date(value).toISOString(); }
  else if (typeof value !== "string" || (node.required && !value.trim())) return fail("请填写{path}");
  if (node.kind === "select" && !node.options?.some(([option]) => option === value)) return fail("{path}请选择有效选项");
  return value;
}

function validateMetering(capabilities: Record<string, JSONValue>[], prefix: string): void {
  if (!capabilities.length) throw new ConfigurationError("至少需要一项计费。", `${prefix}.capabilities`);
  capabilities.forEach((capability, index) => {
    const path = `${prefix}.capabilities.${index}`;
    const bounds = (capability.billable_bounds || {}) as Record<string, JSONValue>, rules = (capability.usage_rules || []) as Record<string, JSONValue>[];
    if (!rules.length) throw new ConfigurationError("至少需要一个可靠的计量组件及对应有限用量上界。", `${path}.usage_rules`);
    const seen = new Set<string>();
    rules.forEach((rule, ruleIndex) => {
      const component = String(rule.component);
      if (seen.has(component)) throw new ConfigurationError("计量组件标识不能重复。", `${path}.usage_rules.${ruleIndex}.component`);
      seen.add(component);
      if (bounds[component] === undefined) throw new ConfigurationError("每个计量组件都须有对应的有限用量上界。", `${path}.billable_bounds`);
    });
    if (Object.keys(bounds).length !== seen.size) throw new ConfigurationError("用量上界与计量组件须一一对应。", `${path}.billable_bounds`);
  });
}
export function validateModelConfiguration(value: JSONValue): void {
  const root = value as Record<string, JSONValue>;
  if (root.supplier_model && typeof root.supplier_model === "object" && !Array.isArray(root.supplier_model)) {
    const model = root.supplier_model as Record<string, JSONValue>;
    const capabilities = (model.capabilities || []) as Record<string, JSONValue>[];
    validateMetering(capabilities, "supplier_model");
    const priced = new Map<string, string>();
    ((model.prices || []) as Record<string, JSONValue>[]).forEach((price, index) => {
      const key = String(price.key || "");
      if (!key || priced.has(key)) throw new ConfigurationError("每一项成本价都要对应一个不重复的计费名称。", `supplier_model.prices.${index}.key`);
      priced.set(key, String(price.unit || ""));
    });
    capabilities.forEach(capability => ((capability.usage_rules || []) as Record<string, JSONValue>[]).forEach(rule => {
      if (priced.get(String(rule.component)) !== String(rule.unit || "")) throw new ConfigurationError("成本价的名称和单位要与计费一致。", "supplier_model.prices");
    }));
    return;
  }
  if (root.route && typeof root.route === "object" && !Array.isArray(root.route)) {
    const seen = new Set<string>();
    (((root.route as Record<string, JSONValue>).bindings || []) as Record<string, JSONValue>[]).forEach((binding, index) => {
      const id = String(binding.supplier_model_id || "");
      if (!id) throw new ConfigurationError("请选择供应商模型。", `route.bindings.${index}.supplier_model_id`);
      if (seen.has(id)) throw new ConfigurationError("同一个供应商模型只要绑定一次。", `route.bindings.${index}.supplier_model_id`);
      seen.add(id);
    });
    return;
  }
  const model = root.model as Record<string, JSONValue> | undefined;
  if (!model) return;
  const capabilities = (model.capabilities || []) as Record<string, JSONValue>[];
  if (model.enabled && !capabilities.length) throw new ConfigurationError("开放模型前，至少勾选一项能力。", "model.capabilities");
  const seen = new Set<string>();
  capabilities.forEach((capability, index) => {
    const operation = String(capability.operation || "");
    if (!operation) throw new ConfigurationError("请选择调用能力。", `model.capabilities.${index}.operation`);
    if (seen.has(operation)) throw new ConfigurationError("同一种调用能力只需要勾选一次。", `model.capabilities.${index}.operation`);
    seen.add(operation);
  });
}

// These checks mirror the published price contract without converting asset
// amounts to Number. The server remains authoritative for scope and notice rules.
export function validatePriceConfiguration(value: JSONValue): void {
  const root = value as Record<string, JSONValue>, price = root.price as Record<string, JSONValue> | undefined;
  if (!price) return;
  if (price.published === false) throw new ConfigurationError("请确认发布此价格版本后再提交。", "price.published");
  if (price.layer === "promotion" && !price.ends_at) throw new ConfigurationError("限时促销必须填写价格结束时间。", "price.ends_at");
  if (price.ends_at && new Date(String(price.ends_at)).getTime() <= new Date(String(price.effective_at)).getTime()) throw new ConfigurationError("价格结束时间必须晚于生效时间。", "price.ends_at");
  const rule = price.rule as Record<string, JSONValue>;
  if (!rule) throw new ConfigurationError("请填写计价方式。", "price.rule");
  if (price.model_id === "*" && (price.layer !== "regular" || rule.mode !== "follow")) throw new ConfigurationError("全部模型的默认规则仅适用于常规自动跟随价格。", "price.model_id");
  for (const [scopePath, scope] of [["price.scope", price.scope], ["price.rule.follow", rule.mode === "follow" ? rule.follow : undefined]] as const) {
    if (!scope || typeof scope !== "object" || Array.isArray(scope)) continue;
    if ((scope.kind === "supply" || scope.kind === "supplier") && scope.seller_id === scope.buyer_id) throw new ConfigurationError("买方经营方或上游路由不能与卖方相同。", `${scopePath}.buyer_id`);
  }
  if (rule.mode === "follow") {
    const scope = price.scope as Record<string, JSONValue> | undefined, follow = rule.follow as Record<string, JSONValue> | undefined;
    if (scope?.kind === "supplier") throw new ConfigurationError("供应商成本价须逐组件手动定价。", "price.rule.mode");
    if (follow?.kind && follow.kind !== "supply") throw new ConfigurationError("自动跟随须使用直接上级对本经营方的供货价。", "price.rule.follow.kind");
    if (scope && follow && ["kind", "seller_id", "buyer_id"].every(key => (scope[key] || "") === (follow[key] || ""))) throw new ConfigurationError("自动跟随的价目不能是本次定价范围本身。", "price.rule.follow");
    if (compareDecimal(String(rule.multiplier), "0") <= 0) throw new ConfigurationError("自动跟随倍率必须大于 0。", "price.rule.multiplier");
    for (const [key, multiplier] of Object.entries((rule.model_multipliers || {}) as Record<string, string>)) {
      if (key === "*") throw new ConfigurationError("指定模型倍率须填写实际模型；默认倍率请使用上方字段。", `price.rule.model_multipliers.${key}`);
      if (compareDecimal(multiplier, "0") <= 0) throw new ConfigurationError("指定模型的倍率必须大于 0。", `price.rule.model_multipliers.${key}`);
    }
    return;
  }
  const components = (rule.components || []) as Record<string, JSONValue>[];
  if (!components.length) throw new ConfigurationError("手动定价至少需要一个计费组件。", "price.rule.components");
  const seen = new Set<string>();
  components.forEach((component, index) => {
    const path = `price.rule.components.${index}`;
    if (seen.has(String(component.key))) throw new ConfigurationError("同一价格中，计费组件标识不能重复。", `${path}.key`);
    seen.add(String(component.key));
    if (compareDecimal(String(component.per), "0") <= 0) throw new ConfigurationError("每多少单位必须大于 0。", `${path}.per`);
    const tiers = (component.tiers || []) as Record<string, JSONValue>[];
    let from = "0";
    tiers.forEach((tier, tierIndex) => {
      const tierPath = `${path}.tiers.${tierIndex}`;
      if (compareDecimal(String(tier.from), from) !== 0) throw new ConfigurationError("阶梯价格应从 0 开始，且与上一阶结束数量连续衔接。", `${tierPath}.from`);
      if (tierIndex === tiers.length - 1 && tier.to) throw new ConfigurationError("最后一阶的结束数量请留空，覆盖剩余用量。", `${tierPath}.to`);
      if (tierIndex < tiers.length - 1 && !tier.to) throw new ConfigurationError("中间阶梯必须填写结束数量。", `${tierPath}.to`);
      if (tier.to && compareDecimal(String(tier.to), String(tier.from)) <= 0) throw new ConfigurationError("阶梯结束数量必须大于起始数量。", `${tierPath}.to`);
      from = String(tier.to || "0");
    });
  });
}
