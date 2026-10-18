import type { Entity, JSONValue } from "./contracts";
import { translate, type Locale } from "./i18n";

const opaqueID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const relationLabels: Record<string, string> = { requested_by:"申请人",created_by:"登记人",approved_by:"审核人",verified_by:"核实人",subject_id:"关联业务主体", customer_id: "关联客户", owner_id: "关联主体", supplier_id: "关联供货方", buyer_id: "关联采购方", operator_id: "关联经营方", brand_id: "关联品牌", order_id: "关联订单", receipt_id: "关联收款", package_id: "关联套餐", plan_id: "关联套餐", staff_id: "经营人员", credential_id: "已保存的供应商凭据", contract_attachment_id: "私有合作合同" };

export function resourceColumnVisible(key: string, resource?: string): boolean {
  return (key !== "action" || resource === "restrictions") && key !== "version" && key !== "expected_version" && !key.endsWith("_version") && key !== "staff_id";
}

/** Labels are display-only; never replace IDs in saved records or commands. */
export function readableResourceValue(row: Entity, field: string, locale: Locale = "zh"): JSONValue | undefined {
  const label = row.presentation?.labels[field];
  if (label) return label;
  const value = row.values[field];
  if (typeof value === "string" && opaqueID.test(value) && (field.endsWith("_id") || field.endsWith("_by") || field === "reference")) return translate(locale,relationLabels[field] || "关联记录");
  if (field === "purpose" && typeof value === "string" && value.startsWith("customer:atg:")) return translate(locale,"客户 ATG 权益");
  return value;
}
