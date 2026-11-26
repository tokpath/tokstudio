export type CustomerOrderSection = "orders" | "topups";

export function customerOrderSection(kind: string): CustomerOrderSection {
  return kind === "topup" ? "topups" : "orders";
}

export function customerOrderRecordHref(kind: string, id: string): string {
  return `/workspace/${customerOrderSection(kind)}?record=${encodeURIComponent(id)}`;
}
