export type WalletOperation = {id:string;adapter:string;payMajor:number;orderId?:string};
export function restoreWalletOperation(raw:string|null):WalletOperation|null {
  try {
    const value=raw ? JSON.parse(raw) : null;
    if (!value || typeof value.id!=="string" || !value.id || value.id.length>128 || typeof value.adapter!=="string" || !value.adapter || !Number.isSafeInteger(value.payMajor) || value.payMajor<=0) return null;
    return {id:value.id,adapter:value.adapter,payMajor:value.payMajor,...(typeof value.orderId==="string" && value.orderId ? {orderId:value.orderId} : {})};
  } catch {return null;}
}
