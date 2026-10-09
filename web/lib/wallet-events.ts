export type WalletScope = { userId: string; brandId: string };
const walletChangedEvent = "tokenhub:wallet-changed";
export function notifyWalletChanged(scope: WalletScope) {
  if (scope.userId) window.dispatchEvent(new CustomEvent<WalletScope>(walletChangedEvent, { detail: scope }));
}
export function subscribeWalletChanged(scope: WalletScope, refresh: () => void) {
  const listener = (event: Event) => {
    const changed = (event as CustomEvent<WalletScope>).detail;
    if (scope.userId && changed?.userId === scope.userId && changed.brandId === scope.brandId) refresh();
  };
  window.addEventListener(walletChangedEvent, listener);
  return () => window.removeEventListener(walletChangedEvent, listener);
}
