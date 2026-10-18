"use client";
import { useEffect, useState } from "react";

const eventName = "tokenhub:before-navigation";
type PendingNavigation = { url: string; state: unknown; blocked: () => void };
type TraverseEvent = Event & { navigationType?: string };
const pending = new Set<PendingNavigation>();
let installed = false;

function notifyBlocked() { for (const guard of pending) guard.blocked(); }
function installNavigationGuard() {
  if (installed || typeof window === "undefined") return;
  installed = true;
  const prevent = (event: Event) => { if (!pending.size) return; event.preventDefault(); notifyBlocked(); };
  window.addEventListener(eventName, prevent);
  window.addEventListener("beforeunload", event => { if (pending.size) { event.preventDefault(); event.returnValue = ""; } });
  document.addEventListener("click", event => {
    if (!pending.size) return;
    const link = event.target instanceof Element ? event.target.closest("a[href]") : null;
    if (!(link instanceof HTMLAnchorElement) || link.target === "_blank" || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || link.getAttribute("href")?.startsWith("#")) return;
    prevent(event); event.stopPropagation();
  }, true);

  // Cancel traversal before its history entry or React tree changes. In
  // browsers without Navigation API, this module-level listener is installed
  // before Next's mounted popstate listener, so it preserves the same form.
  const navigation = (window as Window & { navigation?: EventTarget }).navigation;
  navigation?.addEventListener("navigate", raw => {
    const event = raw as TraverseEvent;
    if (pending.size && event.navigationType === "traverse" && event.cancelable) prevent(event);
  });
  window.addEventListener("popstate", event => {
    const guard = Array.from(pending).at(-1);
    if (!guard) return;
    event.stopImmediatePropagation();
    window.history.pushState(guard.state, "", guard.url);
    notifyBlocked();
  }, true);
}

// Register before the router's mount effects. Adding this listener only when an
// action becomes pending is too late: Next can synchronously unmount that action
// during popstate before the late listener is reached.
installNavigationGuard();

export function allowNavigation() {
  installNavigationGuard();
  return window.dispatchEvent(new Event(eventName, { cancelable: true }));
}

export function useNavigationBlock(active: boolean) {
  const [blocked, setBlocked] = useState(false);
  useEffect(() => {
    installNavigationGuard();
    if (!active) { setBlocked(false); return; }
    const guard = { url: window.location.href, state: window.history.state, blocked: () => setBlocked(true) };
    pending.add(guard);
    return () => { pending.delete(guard); };
  }, [active]);
  return blocked;
}
