"use client";

import { useTranslations } from "next-intl";
import { useEffect, useRef, useState } from "react";
import { ConfirmDialog } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { formatUsdMinor, parseUsdToMinor } from "@/lib/money";
import { useViewer } from "@/components/rbac/viewer-context";

type Recipient = { id: string; email: string; display_name: string; channel_code: string; status: string };
type Operation = { key: string; recipient: Recipient; amount: number };

export function BonusPanel() {
  const tUI = useTranslations();
  const viewer = useViewer();
  const storageKey = `bonus-pending:${viewer.userId}`;
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Recipient[]>([]);
  const [selected, setSelected] = useState<Recipient | null>(null);
  const [amount, setAmount] = useState("1");
  const [searching, setSearching] = useState(false);
  const [searched, setSearched] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [operation, setOperation] = useState<Operation | null>(null);
  const [open, setOpen] = useState(false);
  const [sending, setSending] = useState(false);
  const searchVersion = useRef(0);
  const inFlight = useRef(false);
  const minor = parseUsdToMinor(amount);

  useEffect(() => {
    try {
      const saved = sessionStorage.getItem(storageKey);
      if (saved) {
        const pending = JSON.parse(saved) as Operation;
        if (pending.key && pending.recipient?.id && pending.amount > 0) {
          setOperation(pending);
          setMessage(tUI("consoleCopy.aGiftGrantIsAwaitingConfirmationCheckThe"));
        }
      }
    } catch { /* A corrupt draft must not submit automatically. */ }
  }, [storageKey]);

  async function search() {
    const version = ++searchVersion.current;
    setSearching(true); setError(""); setSelected(null); setResults([]); setSearched(false);
    try {
      const response = await fetch(`${apiBase}/admin/billing/users?q=${encodeURIComponent(query.trim())}`, { credentials: "include" });
      const body = await response.json();
      if (version !== searchVersion.current) return;
      if (!response.ok) throw new Error(body.error?.message || tUI("consoleCopy.searchFailedPleaseTryAgain"));
      setResults(body.items || []); setSearched(true);
    } catch (e) {
      if (version === searchVersion.current) setError(e instanceof Error ? e.message : tUI("consoleCopy.searchFailedPleaseTryAgain"));
    } finally { if (version === searchVersion.current) setSearching(false); }
  }

  function review() {
    setError("");
    if (!operation && selected && minor !== null && minor > 0) {
      setOperation({ key: crypto.randomUUID(), recipient: selected, amount: minor });
    }
    setOpen(true);
  }

  async function grant() {
    if (!operation || inFlight.current) return false;
    // Save before sending, so a lost response or page reload retries the same operation.
    try { sessionStorage.setItem(storageKey, JSON.stringify(operation)); }
    catch { setError(tUI("consoleCopy.theBrowserCouldNotSaveTheOperationRecord")); return false; }
    inFlight.current = true; setSending(true); setError("");
    try {
      const response = await fetch(`${apiBase}/admin/entitlements/bonus`, {
        method: "POST", credentials: "include",
        headers: { ...confirmHeaders, "Idempotency-Key": operation.key },
        body: JSON.stringify({ user_id: operation.recipient.id, unit_type: "usd_credit", amount: operation.amount, expires_in_seconds: 86400 }),
      });
      const body = await response.json();
      if (!response.ok) {
        setError(body.error?.message || tUI("consoleCopy.theGrantIsIncompleteRetryToCheckThe"));
        // Validation/auth failures are definitive. Conflicts and server failures retain the operation.
        if ([400, 401, 403].includes(response.status)) { sessionStorage.removeItem(storageKey); }
        return false;
      }
      sessionStorage.removeItem(storageKey);
      setMessage(tUI("consoleCopy.grantedUsdToTheUserCanCheckRemaining", { value0: String(operation.recipient.email), value1: String(formatUsdMinor(operation.amount)) }));
      setOperation(null); setSelected(null); setResults([]); setSearched(false); setQuery("");
      return true;
    } catch {
      setError(tUI("consoleCopy.theGrantResultIsNotYetConfirmedRetry"));
      return false;
    } finally { inFlight.current = false; setSending(false); }
  }

  function close(next: boolean) {
    setOpen(next);
    if (!next && !inFlight.current) {
      try { if (!sessionStorage.getItem(storageKey)) setOperation(null); } catch { /* Keep the draft. */ }
    }
  }

  return <section className="my-5 space-y-3 rounded-control border border-hairline p-4" aria-label={tUI("consoleCopy.grantGiftCredit")}>
    <h3 className="font-semibold">{tUI("consoleCopy.grantGiftCredit")}</h3>
    <p className="text-sm text-ink-secondary">{tUI("consoleCopy.expires24HoursAfterTheGrantAndIs")}</p>
    <form className="flex flex-wrap items-end gap-2" onSubmit={e => { e.preventDefault(); if (query.trim().length >= 2 && !operation) void search(); }}>
      <div className="min-w-64 flex-1"><Label htmlFor="bonus-user-search">{tUI("consoleCopy.recipient")}</Label>
        <Input id="bonus-user-search" placeholder={tUI("consoleCopy.enterAnEmailNameOrUserId")} value={query} maxLength={200} disabled={!!operation}
          onChange={e => { ++searchVersion.current; setQuery(e.target.value); setSelected(null); setResults([]); setSearched(false); setSearching(false); setError(""); }} />
      </div>
      <Button type="submit" variant="outline" disabled={query.trim().length < 2 || searching || !!operation}>{searching ? tUI("consoleCopy.searching") : tUI("consoleCopy.searchUsers")}</Button>
    </form>
    {searched && !results.length ? <p role="status">{tUI("consoleCopy.noUsersFoundCheckTheEmailOrEnter")}</p> : null}
    {results.length ? <div><p className="text-sm text-ink-secondary">{tUI("consoleCopy.selectARecipientUpTo20UsersAre")}</p>
      <ul className="mt-2 space-y-2">{results.map(user => <li key={user.id}>
        <Button className="h-auto w-full justify-start whitespace-normal text-left" variant={selected?.id === user.id ? "default" : "outline"}
          disabled={user.status !== "active" || !!operation} type="button" onClick={() => { setSelected(user); setMessage(""); }} aria-pressed={selected?.id === user.id}>
          {user.display_name || tUI("consoleCopy.nameNotSet")} · {user.email} {tUI("consoleCopy.channel")}{user.channel_code || tUI("consoleCopy.unassigned")}{user.status !== "active" ? tUI("consoleCopy.disabled") : ""}
        </Button>
      </li>)}</ul></div> : null}
    <div><Label htmlFor="bonus-usd">{tUI("consoleCopy.giftAmountUsd")}</Label><Input id="bonus-usd" className="w-48" inputMode="decimal" value={amount} disabled={!!operation} onChange={e => setAmount(e.target.value)} aria-describedby="bonus-help" />
      <p id="bonus-help" className="mt-1 text-xs text-ink-secondary">{tUI("consoleCopy.greaterThan0UpTo6DecimalPlaces")}</p></div>
    <Button type="button" size="sm" onClick={review} disabled={sending || (!operation && (!selected || minor === null || minor <= 0))}>{operation ? tUI("serviceQuota.lookup") : tUI("consoleCopy.grantGiftCredit")}</Button>
    {error && !open ? <p role="alert" className="text-sm text-danger">{error}</p> : null}
    {message ? <p role="status" className="text-sm">{message}</p> : null}
    <ConfirmDialog open={open} onOpenChange={close} title={tUI("consoleCopy.confirmGiftCreditGrant")} error={error} onConfirm={grant}
      description={operation ? tUI("consoleCopy.grantUsdToChannelFor24HoursThe", { value0: String(operation.recipient.display_name || tUI("chrome.groupUser")), value1: String(operation.recipient.email), value2: String(operation.recipient.channel_code || tUI("consoleCopy.unassigned")), value3: String(formatUsdMinor(operation.amount)) }) : ""} />
  </section>;
}
