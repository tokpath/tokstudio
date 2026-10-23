import { Decimal, type JSONValue, type Operation } from "@/lib/contracts";
import { compareDecimal } from "@/lib/decimal";
import { translate, type Locale } from "@/lib/i18n";

export type ConfigNode = {
  label: string;
  kind: "text" | "textarea" | "decimal" | "integer" | "boolean" | "datetime" | "select" | "object" | "array" | "record";
  help?: string;
  required?: boolean;
  default?: JSONValue;
  generated?: "uuid";
  fields?: Record<string, ConfigNode>;
  item?: ConfigNode;
  options?: [string, string][];
  when?: { field: string; values: string[] };
  lookup?: "models" | "credentials" | "buyers" | "suppliers" | "routes" | "supplier-models" | "pricing-models";
  lookupBy?: { field: string; values: Record<string, "buyers" | "routes"> };
  lookupWhen?: { field: string; values: string[] };
  wildcard?: boolean;
  signed?: boolean;
  min?: number;
  max?: number;
  nullable?: boolean;
  readOnly?: boolean;
  /** Capability cards explain call, billing, and verification before the rare limits. */
  presentation?: "operations" | "price";
  /** Drawn by a custom editor. The value is still kept and submitted. */
  hidden?: boolean;
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
const capability: ConfigNode = object("供应商能力", {
  operation: select("调用能力", operations.filter(([key]) => key !== "video_retrieve" && key !== "video_download")),
  streaming: { ...boolean("支持流式输出"), help: "选择是时，同时支持普通调用与流式调用。用量由系统适配读取。" },
});
const scope = (label: string, operatorID?: string) => object(label, {
  kind: select("价目用途", [["retail", "客户零售价"], ["supply", "对下级供货价"]], "retail"),
  seller_id: { ...text("定价经营方（自动带入）"), default: operatorID, readOnly: true },
  buyer_id: { ...text("供货采购方"), lookupBy: {field:"kind",values:{supply:"buyers"}}, when: { field: "kind", values: ["supply"] } },
});
const tier = object("阶梯", { from: decimal("起始数量（含）"), to: decimal("结束数量（不含，最后一阶可留空）", false), rate: decimal("本阶梯单价") });
const component: ConfigNode = { ...object("计费项目", {
  key: { ...text("计费项目"), readOnly: true },
  label: { ...text("计费项目名称", false), readOnly: true },
  unit: { ...text("计量单位"), readOnly: true },
  per: { ...decimal("计价数量"), readOnly: true },
  rate: decimal("ATG 单价"),
  tiers: array("渐进阶梯价格（可选）", tier),
}), presentation: "price" };

const billingTemplates: [string,string][] = [["tokens","按 Token"],["images","按张"],["megapixels","按百万像素"],["characters","按字符"],["seconds","按秒"]];
function billingItem(direction:"input"|"output"):ConfigNode{return object("计量项目",{
 modality:select("计量范围",[["all","全部模态（统一 Token）"],["text","文本"],["image","图片"],["audio","音频"],["video","视频"]],"all"),
 template:select("计费模板",billingTemplates,"tokens"),
 variants:{...array("规格档位（可选）",object("规格",{
  id:{...text("规格标识"),generated:"uuid",readOnly:true},name:text("客户看到的规格名称"),size:text("尺寸（可选）",false,"例如 1024x1024。输入图片按实际尺寸识别，输出规格也会核对实际结果。"),
  ...(direction==="output"?{quality:{...select("质量（可选）",[["","不区分"],["standard","标准"],["hd","高清"],["high","高"],["medium","中"],["low","低"]],""),required:false}}:{}),
 }),"不分档时留空。不需要填写判断公式。"),when:{field:"modality",values:direction==="input"?["image"]:["image","video"]},hideWhen:{field:"template",values:["tokens"]}},
})}


export function configurationSchema(operation: Operation, operatorID?: string): ConfigNode | undefined {
  if (/\/catalog$/.test(operation.path)) return object("公开模型", {
    model: object("模型名称与能力", {
      id: text("公开模型标识", true, "客户请求和页面都使用这个标识，例如 demo-chat。创建后不能修改。"),
      capabilities: { ...array("可调用的能力", object("能力", { operation: select("调用能力", operations.filter(([key]) => key !== "video_retrieve" && key !== "video_download")) }), "勾选客户可以调用的能力。计费和供应商价格写在供应商模型上。是否对客户开放，在列表里切换。"), presentation: "operations", options: operations.filter(([key]) => key !== "video_retrieve" && key !== "video_download") },
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
      provider: select("接口协议", [["openai", "OpenAI 兼容"], ["anthropic", "Claude"], ["gemini", "Gemini"], ["openrouter", "OpenRouter"], ["elevenlabs", "ElevenLabs"]], "openai"),
      model: text("供应商模型名称", true, "供应商要求填写的模型名。客户请求里的名字写在公开模型上。"),
      enabled: { ...boolean("启用这个供应商模型"), default: true },
      base_url: { ...text("API Base URL", false, "官方供应商通常留空。兼容服务填写供应商公布的 API Base URL，具体调用路径由系统生成。"), when: {field:"provider",values:["openai","anthropic","openrouter"]} },
      pricing_reference: { ...text("Bifrost 参考计价模型", false), lookup: "pricing-models", help: "选择参考价目中的供应商与模型，用于系统核算成本。未匹配的成本会标为待核实，不会记成免费。" },
      cost_provider: { ...text("参考价目供应商", false), readOnly: true },
      cost_model: { ...text("参考计价模型", false), readOnly: true },
      capabilities: array("供应商支持的能力", capability, "勾选供应商实际支持的能力。计量规则由系统适配，参考成本由 Bifrost 计算。"),
    }, "选择供应商连接与模型，确认实际能力，并选择用于成本核算的参考计价模型。"),
    expected_version: { ...integer("当前记录版本（自动带入）", 0), readOnly: true },
  });
  if (/\/routes$/.test(operation.path)) return object("路由配置", {
    route: object("公开模型的路由", {
      public_model_id: { ...text("公开模型"), lookup: "models" },
      strategy: select("选路策略", [["priority", "固定优先级"], ["lowest_latency", "最低延迟"], ["lowest_cost", "预估最低成本"]], "priority"),
      bindings: array("绑定的供应商模型", object("绑定", {
        id: { ...text("绑定标识"), generated: "uuid", readOnly: true },
        supplier_model_id: { ...text("供应商模型"), lookup: "supplier-models" },
        priority: { ...integer("选路优先级（数字越小越优先）", 0), signed: true },
        enabled: { ...boolean("这条绑定接单"), default: true },
        routing: { ...object("熔断", { max_concurrent: { ...integer("最大同时在途（0 不额外限制）", 0), max: 100000 }, failure_threshold: { ...integer("连续失败阈值（0 使用默认 3）", 0), max: 100 }, unknown_usage_threshold: { ...integer("未知用量阈值（0 使用默认 2）", 0), max: 100 }, recovery_successes: { ...integer("恢复成功探测次数（0 使用默认 2）", 0), max: 20 }, cooldown_seconds: { ...integer("冷却秒数（0 使用默认 60）", 0), max: 86400 } }), required: false },
      }), "一个公开模型可以绑多个供应商模型。数字小的优先级先被考虑。"),
    }, "选择先用哪一条，并写下连续失败后暂时停用的数字。"),
    expected_version: { ...integer("当前策略版本（自动带入）", 0), readOnly: true },
  });
  if (/\/pricing$/.test(operation.path) && operation.name === "template") return object("模型计费模板", {
    action: { ...text("操作"), default: "billing", readOnly: true },
    model_id: { ...text("公开模型"), lookup: "models" },
    operation: select("调用能力", operations.filter(([key]) => key !== "video_retrieve" && key !== "video_download")),
    billing: object("输入与输出计量", {
      input: array("输入计量项目", billingItem("input"), "可不单独计收输入费用。统一 Token 与分模态项目不能在同一方向并用。"),
      output: array("输出计量项目", billingItem("output"), "只配置需要计价的输出。语音合成按输入字符计价时，可以不再单列输出费用。"),
    }, "同一份内容只采用一种计量方式。按模态组合时，系统必须能够可靠拆分用量。"),
    expected_version: { ...integer("当前版本", 0), readOnly: true },
    effective_at: datetime("计划生效时间（变更已开放模型时填写）"),
    evidence: { ...text("变更及通知依据", false), kind: "textarea" },
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
      }, "零售价使用客户 ATG，供货价使用采购 ATG。供应商成本由 Bifrost 独立核算。倍率不是利润率。"),
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

// Keep only the fields the form knows. Records keep their named entries.
export function retainKnownFields(value: JSONValue, node: ConfigNode): JSONValue {
  if (node.kind === "object") {
    if (!value || Array.isArray(value) || typeof value !== "object") return value;
    const source = value as Record<string, JSONValue>;
    return Object.fromEntries(Object.entries(node.fields || {}).flatMap(([key, child]) => source[key] === undefined ? [] : [[key, retainKnownFields(source[key], child)]]));
  }
  if (node.kind === "array" && Array.isArray(value)) {
    const items = value.map(item => node.item ? retainKnownFields(item, node.item) : item);
    if (node.options && items.every(item => typeof item === "string")) {
      const allowed = new Set(node.options.map(([key]) => key));
      return items.filter(item => allowed.has(String(item)));
    }
    return items;
  }
  if (node.kind === "record" && value && !Array.isArray(value) && typeof value === "object") return Object.fromEntries(Object.entries(value).map(([key, item]) => [key, node.item ? retainKnownFields(item, node.item) : item]));
  return value;
}

export function nodeVisible(node: ConfigNode, parent: Record<string, JSONValue>): boolean {
  if (node.hideWhen && node.hideWhen.values.includes(String(parent[node.hideWhen.field] ?? ""))) return false;
  return !node.when || node.when.values.includes(String(parent[node.when.field] ?? ""));
}

// Only fields drawn on the form are kept. Anything else is dropped on save.
// Monetary values stay as strings.
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

export function prepareConfiguration(value: JSONValue, node: ConfigNode, path = node.label, fieldPath = "", pathParts: ConfigurationPathPart[] = [{ source: path }]): JSONValue {
  const fail = (message: string, values: Record<string, string | number> = { path }): never => { throw new ConfigurationError(message, fieldPath, values, pathParts); };
  if (value === null && node.nullable) return null;
  if (value === null && !node.required && ["object", "array", "record"].includes(node.kind)) return null;
  if (node.kind === "object") {
    if (!value || Array.isArray(value) || typeof value !== "object") return fail("{path}应为一组业务字段");
    const source = value as Record<string, JSONValue>;
    const out: Record<string, JSONValue> = {};
    for (const [key, child] of Object.entries(node.fields || {})) {
      if (!nodeVisible(child, source)) continue;
      const current = source[key];
      if (current !== undefined && current !== "") out[key] = prepareConfiguration(current, child, `${path} · ${child.label}`, fieldPath ? `${fieldPath}.${key}` : key, [...pathParts, { source: child.label, separator: " · " }]);
      else if (child.required) throw new ConfigurationError(child.readOnly ? "无法核实{label}，请重新选择业务记录后重试。" : "请填写{path} · {label}", child.readOnly && node.fields?.model_id?.lookup ? (fieldPath ? `${fieldPath}.model_id` : "model_id") : (fieldPath ? `${fieldPath}.${key}` : key), { path, label: child.label }, pathParts);
    }
    return out;
  }
  if (node.kind === "array") {
    if (!Array.isArray(value)) return fail("{path}应为列表");
    const items = value.map((item, index) => prepareConfiguration(item, node.item!, `${path} ${index + 1}`, `${fieldPath}.${index}`, [...pathParts, { source: String(index + 1), separator: " ", literal: true }]));
    if (node.options && items.every(item => typeof item === "string")) {
      const allowed = new Set(node.options.map(([key]) => key));
      return items.filter(item => allowed.has(String(item)));
    }
    return items;
  }
  if (node.kind === "record") {
    if (!value || Array.isArray(value) || typeof value !== "object") return fail("{path}应为按名称配置的字段");
    return Object.fromEntries(Object.entries(value).map(([key, item]) => {
      if (!key.trim()) return fail("请填写{path}的项目名称");
      return [key, prepareConfiguration(item, node.item!, `${path} · ${key}`, `${fieldPath}.${key}`, [...pathParts, { source: key, separator: " · ", literal: true }])];
    }));
  }
  if (node.kind === "decimal") { if (typeof value !== "string" || !Decimal.safeParse(value).success) return fail("{path}请输入非负十进制数，最多 24 位小数"); }

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

export function presentSupplierConfiguration(value: JSONValue): JSONValue {
  if (!value || typeof value !== "object" || Array.isArray(value)) return value;
  const root = value as Record<string, JSONValue>, model = root.supplier_model as Record<string, JSONValue>;
  if (!model || !Array.isArray(model.capabilities)) return value;
  const modes = new Map<string, Record<string, JSONValue>>();
  for (const item of model.capabilities) if (item && typeof item === "object" && !Array.isArray(item)) {
    const cap = item as Record<string, JSONValue>, key = String(cap.operation || "");
    const previous = modes.get(key); modes.set(key, { operation: key, streaming: cap.streaming === true || previous?.streaming === true });
  }
  return { ...root, supplier_model: { ...model, capabilities: [...modes.values()] } };
}

export function validateModelConfiguration(value: JSONValue): void {
  const root = value as Record<string, JSONValue>;
  if(root.action==="billing") {
    const billing=root.billing as Record<string,JSONValue>;
    const inputs=Array.isArray(billing?.input)?billing.input:[],outputs=Array.isArray(billing?.output)?billing.output:[];
    if(!inputs.length && !outputs.length)throw new ConfigurationError("至少配置一个输入或输出计量项目。","billing.input");
    for(const direction of ["input","output"] as const){const items=(direction==="input"?inputs:outputs) as Record<string,JSONValue>[];const seen=new Set<string>();
      items.forEach((item,index)=>{const path=`billing.${direction}.${index}`;const modality=String(item.modality);
        if(seen.has(modality))throw new ConfigurationError("同一方向的同一种模态只能配置一个计量项目。",`${path}.modality`);seen.add(modality);
        if(modality==="all" && items.length>1)throw new ConfigurationError("统一 Token 与分模态计费不能在同一方向并用。",`${path}.modality`);
        const variants=Array.isArray(item.variants)?item.variants as Record<string,JSONValue>[]:[];
        variants.forEach((variant,i)=>{if(!variant.size && !variant.quality)throw new ConfigurationError("规格至少填写尺寸或质量。",`${path}.variants.${i}.size`);});
      });
    }
    return;
  }
  if (root.supplier_model && typeof root.supplier_model === "object" && !Array.isArray(root.supplier_model)) {
    const model = root.supplier_model as Record<string, JSONValue>;
    const capabilities = (model.capabilities || []) as Record<string, JSONValue>[];
    if (!capabilities.length) throw new ConfigurationError("至少选择一种供应商能力。", "supplier_model.capabilities");
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
