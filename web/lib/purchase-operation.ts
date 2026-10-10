export type PurchaseOperation = { id: string; planId: string; adapter: string; autoRenew: boolean };
export function restorePurchaseOperation(raw: string | null): PurchaseOperation | null {
  try {
    const item = raw ? JSON.parse(raw) : null;
    if (!item || typeof item.id !== "string" || !item.id || item.id.length > 128 || typeof item.planId !== "string" || !item.planId || typeof item.adapter !== "string" || !item.adapter || typeof item.autoRenew !== "boolean") return null;
    return {id:item.id,planId:item.planId,adapter:item.adapter,autoRenew:item.autoRenew};
  } catch { return null; }
}
export function newPurchaseOperation(planId: string, adapter: string, autoRenew: boolean): PurchaseOperation {
  return { id:crypto.randomUUID(),planId,adapter,autoRenew };
}
