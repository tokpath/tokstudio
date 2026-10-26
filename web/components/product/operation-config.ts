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
  lookupPurpose?: "routing";
  lookupBy?: { field: string; values: Record<string, "buyers" | "routes"> };
  lookupWhen?: { field: string; values: string[] };
  wildcard?: boolean;
  signed?: boolean;
  min?: number;
  max?: number;
  nullable?: boolean;
  readOnly?: boolean;
  /** Capability cards explain call, billing, and verification before the rare limits. */
  presentation?: "operations" | "supplier-operations" | "routes" | "price" | "billing";
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
const resolutionPrice = object("分辨率价格", {
  size: text("分辨率", true, "例如 1024x1024 或 1280x720"),
  rate: decimal("该分辨率的 ATG 单价"),
});
const component: ConfigNode = { ...object("计费项目", {
  key: { ...text("计费项目"), readOnly: true },
  label: { ...text("计费项目名称", false), readOnly: true },
  unit: { ...text("计量单位"), readOnly: true },
  per: { ...decimal("计价数量"), readOnly: true },
  rate: decimal("ATG 单价"),
  resolutions: { ...array("分辨率价格（可选）", resolutionPrice, "图片和视频可以按分辨率单独标价。写在这里的尺寸按这一行的单价计费；没有单独标价的尺寸，按上面的单价计费。"), required: false },
}), presentation: "price" };

// Image and video meters can carry one rate per frame size. Audio seconds and token meters cannot.
export function resolutionPriceable(key: string, unit: string) {
  return unit === "image" || unit === "megapixel" || (unit === "second" && key.includes("video"));
}
export function canonicalResolution(value: string): string | undefined {
  const match = /^(\d+)[xX](\d+)$/.exec(value.trim());
  if (!match) return;
  const width = Number(match[1]), height = Number(match[2]);
  if (!Number.isSafeInteger(width) || !Number.isSafeInteger(height) || width < 1 || height < 1 || width > 65536 || height > 65536) return;
  if (String(width) !== match[1] || String(height) !== match[2]) return;
  return `${width}x${height}`;
}
export function groupResolutionPrices(components: Record<string, JSONValue>[]): Record<string, JSONValue>[] {
  const kept: Record<string, JSONValue>[] = [];
  const extras = new Map<string, Record<string, JSONValue>[]>();
  for (const item of components) {
    const key = String(item.key || "");
    const cut = key.indexOf(":");
    const size = cut > 0 ? canonicalResolution(key.slice(cut + 1)) : undefined;
    if (size && !key.slice(cut + 1).includes(":") && resolutionPriceable(key.slice(0, cut), String(item.unit || ""))) {
      const parent = key.slice(0, cut);
      extras.set(parent, [...(extras.get(parent) || []), { size, rate: item.rate ?? "" }]);
      continue;
    }
    kept.push({ ...item, resolutions: Array.isArray(item.resolutions) ? item.resolutions : [] });
  }
  for (const [parent, rows] of extras) {
    const found = kept.find(item => item.key === parent);
    if (found) { found.resolutions = rows; continue; }
    const sample = components.find(item => String(item.key || "").startsWith(`${parent}:`));
    const parts = String(sample?.label || parent).split(" · ");
    const suffix = canonicalResolution(parts[parts.length - 1] || "");
    kept.push({ key: parent, label: suffix ? parts.slice(0, -1).join(" · ") : String(sample?.label || parent), unit: sample?.unit || "", per: sample?.per || "1", rate: "", resolutions: rows });
  }
  return kept;
}

const billingTemplates: [string,string][] = [["tokens","按 Token"],["images","按张"],["megapixels","按百万像素"],["characters","按字符"],["seconds","按秒"]];
function billingItem(direction:"input"|"output"):ConfigNode{return object("计量项目",{
 modality:select("计量范围",[["all","全部模态（统一 Token）"],["text","文本"],["image","图片"],["audio","音频"],["video","视频"]],"all"),
 template:select("计费模板",billingTemplates,"tokens"),
 variants:{...array("规格档位（可选）",object("规格",{
  id:{...text("规格标识"),generated:"uuid",readOnly:true},name:text("客户看到的规格名称"),size:text("分辨率（可选）",false,"例如 1024x1024 或 1280x720。留空表示这一档不限制分辨率。输入按文件的实际尺寸匹配，输出按结果尺寸核对。"),
  ...(direction==="output"?{quality:{...select("质量（可选）",[["","不区分"],["standard","标准"],["hd","高清"],["high","高"],["medium","中"],["low","低"]],""),required:false}}:{}),
 }),"图片和视频可以按分辨率分档，每档单独定价。不需要分档就留空。"),when:{field:"modality",values:["image","video"]},hideWhen:{field:"template",values:["tokens"]}},
})}


export function configurationSchema(operation: Operation, operatorID?: string): ConfigNode | undefined {
  if (/\/catalog$/.test(operation.path)) return object("公开模型", {
    model: object("模型名称与能力", {
      id: text("公开模型标识", true, "客户请求和页面都使用这个标识，例如 demo-chat。创建后不能修改。"),
      capabilities: { ...array("可调用的能力", object("能力", { operation: select("调用能力", operations.filter(([key]) => key !== "video_retrieve" && key !== "video_download")) }), "勾选客户可以调用的能力。计费模板和零售、批发 ATG 价格在「模型与定价」设置；供应商模型上的参考价目仅用于核算上游成本。是否对客户开放，在列表里切换。"), presentation: "operations", options: operations.filter(([key]) => key !== "video_retrieve" && key !== "video_download") },
    }),
    expected_version: { ...integer("当前记录版本（自动带入）", 0), readOnly: true },
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
      capabilities: { ...array("供应商支持的能力", capability, "勾选供应商实际支持的能力；支持流式的能力可以单独开启。"), presentation: "supplier-operations", options: operations.filter(([key]) => key !== "video_retrieve" && key !== "video_download") },
    }, "选择供应商连接与模型，确认实际能力，并选择用于成本核算的参考计价模型。"),
    expected_version: { ...integer("当前记录版本（自动带入）", 0), readOnly: true },
  });
  if (/\/routes$/.test(operation.path)) return object("路由配置", {
    route: object("公开模型的路由", {
      public_model_id: { ...text("公开模型"), lookup: "models", lookupPurpose: "routing" },
      strategy: select("选路策略", [["priority", "按列表顺序优先"], ["lowest_latency", "最低延迟"], ["lowest_cost", "预估最低成本"]], "priority"),
      bindings: { ...array("绑定的供应商模型", object("绑定", {
        id: { ...text("绑定标识"), generated: "uuid", readOnly: true },
        supplier_model_id: { ...text("供应商模型"), lookup: "supplier-models" },
        enabled: { ...boolean("这条绑定接单"), default: true },
      }), "排在前面的优先使用；也作为延迟或成本相同时的选择顺序。故障暂停与恢复由系统自动处理。"), presentation: "routes" },
    }, "选择公开模型后会带入已有路由。只需选择供应商模型、调整顺序和选路策略。"),
    expected_version: { ...integer("当前策略版本（自动带入）", 0), readOnly: true },
  });
  if (/\/pricing$/.test(operation.path) && operation.name === "template") return object("模型计费模板", {
    action: { ...text("操作"), default: "billing", readOnly: true },
    model_id: { ...text("公开模型"), lookup: "models" },
    operation: select("调用能力", operations.filter(([key]) => key !== "video_retrieve" && key !== "video_download")),
    billing: { ...object("输入与输出计量", {
      input: array("输入计量项目", billingItem("input"), "同一能力的输入可以按模态分开定价，例如语音生成的输入文本、图片生成的输入图片。选择全部模态时，文本和图片共用一个 Token 单价，不能再单列图片。"),
      output: array("输出计量项目", billingItem("output"), "只配置需要计价的输出。语音合成按输入字符计价时，可以不再单列输出费用。图片和视频可以按分辨率分档。"),
    }, "同一份内容只采用一种计量方式。按模态组合时，系统必须能够可靠拆分用量。"), presentation: "billing" },
    expected_version: { ...integer("当前版本", 0), readOnly: true },
  });
  if (/\/pricing$/.test(operation.path) && operation.name === "listing") return object("授权与上架", {
    action: select("办理事项", [["listing", "本经营方上架 / 下架"], ["authorization", "授权 / 撤销下级模型权限"]], "listing"),
    buyer_id: { ...text("被授权的直属下级"), lookup: "buyers", when: { field: "action", values: ["authorization"] } }, model_id: { ...text("公开模型"), lookup: "models" }, enabled: boolean("允许使用此模型", true),
    expected_version: { ...integer("当前上架 / 授权版本（自动带入）", 0), required: true, readOnly: true },
  });
  if (/\/pricing$/.test(operation.path)) return object("模型价格", {
    action: { ...text("操作"), default: "price", readOnly: true },
    price: object("价目与适用范围", {
      id: { ...text("本次价格版本标识"), generated: "uuid", readOnly: true }, model_id: { ...text("公开模型"), lookup: "models", wildcard: true }, scope: scope("本次定价范围", operatorID),
      layer: select("价格类别", [["regular", "常规价格"], ["promotion", "限时促销价"]], "regular"), effective_at: { ...datetime("促销开始时间（留空自动安排）"), when: { field: "layer", values: ["promotion"] } }, ends_at: { ...datetime("促销结束时间", true), when: { field: "layer", values: ["promotion"] } },
      rule: object("计价方式", {
        mode: select("计价方式", [["manual", "设置单价"], ["follow", "跟随上级供货价"]], "manual"),
        components: { ...array("组件价格", component), when: { field: "mode", values: ["manual"] } },
        multiplier: { ...{ ...decimal("上级供货价的倍率"), default: "1" }, when: { field: "mode", values: ["follow"] } },
      }, "零售价使用客户 ATG，供货价使用采购 ATG。供应商成本由 Bifrost 独立核算。倍率不是利润率。"),
    }),
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
        const allowed=modality==="all"?["tokens"]:modality==="text"?["tokens","characters"]:modality==="image"?["tokens","images","megapixels"]:["tokens","seconds"];
        if(!["all","text","image","audio","video"].includes(modality) || !allowed.includes(String(item.template)))throw new ConfigurationError("请选择该模态支持的计量方式。",`${path}.template`);
        const variants=Array.isArray(item.variants)?item.variants as Record<string,JSONValue>[]:[];
        variants.forEach((variant,i)=>{if(!variant.size && !variant.quality)throw new ConfigurationError("规格至少填写尺寸或质量。",`${path}.variants.${i}.size`);});
      });
    }
    if(root.operation==="embedding" && outputs.length)throw new ConfigurationError("向量模型仅配置输入计量。","billing.output");
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
  rule.components = expandResolutionPrices((rule.components || []) as Record<string, JSONValue>[]);
  const components = rule.components as Record<string, JSONValue>[];
  if (!components.length) throw new ConfigurationError("手动定价至少需要一个计费组件。", "price.rule.components");
  const seen = new Set<string>();
  components.forEach((component, index) => {
    const path = `price.rule.components.${index}`;
    if (seen.has(String(component.key))) throw new ConfigurationError("同一价格中，计费组件标识不能重复。", `${path}.key`);
    seen.add(String(component.key));
    if (compareDecimal(String(component.per), "0") <= 0) throw new ConfigurationError("每多少单位必须大于 0。", `${path}.per`);
  });
}

// A resolution row becomes its own price component, key:WxH, before the server sees the form.
function expandResolutionPrices(components: Record<string, JSONValue>[]): Record<string, JSONValue>[] {
  const out: Record<string, JSONValue>[] = [];
  components.forEach((component, index) => {
    const resolutions = Array.isArray(component.resolutions) ? component.resolutions as Record<string, JSONValue>[] : [];
    const next = { ...component };
    delete next.resolutions;
    out.push(next);
    const seen = new Set<string>();
    resolutions.forEach((row, rowIndex) => {
      const size = canonicalResolution(String(row?.size || ""));
      const path = `price.rule.components.${index}.resolutions.${rowIndex}.size`;
      if (!size) throw new ConfigurationError("分辨率请写成宽x高，例如 1024x1024。", path);
      if (!resolutionPriceable(String(component.key || ""), String(component.unit || ""))) throw new ConfigurationError("只有图片和视频可以按分辨率定价。", path);
      if (seen.has(size)) throw new ConfigurationError("同一计费项目里，分辨率不能重复。", path);
      seen.add(size);
      const child: Record<string, JSONValue> = { key: `${component.key}:${size}`, unit: component.unit, per: component.per, rate: row.rate };
      if (typeof component.label === "string" && component.label) child.label = `${component.label} · ${size}`;
      out.push(child);
    });
  });
  return out;
}
