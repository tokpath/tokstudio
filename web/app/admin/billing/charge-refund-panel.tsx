"use client";

import { useTranslations } from "next-intl";
import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "next/navigation";
import { safeReturnHref } from "@/lib/return-context";
import Link from "next/link";
import { ConfirmDialog } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
import { formatUsdMinor } from "@/lib/money";

type Preview = { request_id: string; user_id: string; model_id: string; state: string; amount_minor: number; wallet_minor: number; entitlement_minor: number; gift_minor: number };
type Review = { item: Preview; user: { email: string; display_name: string } };
const money = (value: number) => `${formatUsdMinor(value)} USD`;

export function ChargeRefundPanel({ onRefund }: { onRefund: () => void }) {
  const tUI = useTranslations();
  const context = useSearchParams();
  const [requestID, setRequestID] = useState("");
  const [review, setReview] = useState<Review | null>(null);
  const [searching, setSearching] = useState(false);
  const [open, setOpen] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const version = useRef(0);
  const contextRequest = context?.get("request_id") || "";
  useEffect(() => {
    setRequestID(contextRequest); setReview(null); setError(""); setMessage("");
    if(contextRequest) void lookup(contextRequest);
    return () => { version.current++; };
  }, [contextRequest]);

  async function lookup(target = requestID) {
    const current = ++version.current;
    setSearching(true); setReview(null); setError(""); setMessage("");
    try {
      const response = await fetch(`${apiBase}/admin/refunds/preview?request_id=${encodeURIComponent(target.trim())}`, { credentials: "include" });
      const body = await response.json();
      if (current !== version.current) return;
      if (!response.ok) throw new Error(body.error?.message || tUI("consoleCopy.couldNotLoadTheBillPleaseTryAgain"));
      setReview(body);
    } catch (e) {
      if (current === version.current) setError(e instanceof Error ? e.message : tUI("consoleCopy.couldNotLoadTheBillPleaseTryAgain"));
    } finally { if (current === version.current) setSearching(false); }
  }

  async function refund() {
    if (!review) return false;
    setError("");
    try {
      const response = await fetch(`${apiBase}/admin/refunds`, { method: "POST", credentials: "include", headers: confirmHeaders, body: JSON.stringify({ request_id: review.item.request_id }) });
      const body = await response.json();
      if (!response.ok) { setError(body.error?.message || tUI("consoleCopy.theRefundIsIncompletePleaseRetry")); return false; }
      setReview({ ...review, item: { ...review.item, state: "reversed" } });
      setMessage(tUI("consoleCopy.usageRefundForCompletedAndRelatedCommissionsReversed", { value0: String(review.item.request_id), value1: String(money(review.item.wallet_minor)), value2: String(money(review.item.entitlement_minor)) }));
      onRefund();
      return true;
    } catch {
      setError(tUI("consoleCopy.theConnectionWasInterruptedAndTheRefundResult"));
      return false;
    }
  }

  const item = review?.item;
  const person = review ? `${review.user.display_name || tUI("chrome.groupUser")} · ${review.user.email || review.item.user_id}` : "";
  return <section aria-label={tUI("consoleCopy.usageRefund")} className="mb-5 rounded-control border border-hairline p-4">
    <h3 className="font-semibold">{tUI("consoleCopy.usageRefund")}</h3>
    {context?.has("return_to") ? <Link href={safeReturnHref(context?.get("return_to"),"/admin/usage?tab=requests")} className="my-2 inline-block text-sm underline">{tUI("consoleCopy.returnToOriginalRequest")}</Link> : null}
    <p className="my-2 text-sm text-ink-secondary">{tUI("consoleCopy.returnConsumedTopUpCreditToTheWallet")}</p>
    <form className="flex flex-wrap items-end gap-2" onSubmit={e => { e.preventDefault(); if (requestID.trim() && !searching) void lookup(); }}>
      <label className="min-w-0 flex-1 text-sm">{tUI("referral.request")}<Input className="mt-1" value={requestID} disabled={open} placeholder={tUI("consoleCopy.copyTheRequestIdFromUsageBills")} onChange={e => { version.current++; setSearching(false); setRequestID(e.target.value); setReview(null); setError(""); setMessage(""); }} />
      </label>
      <Button type="submit" variant="outline" disabled={!requestID.trim() || searching || open}>{searching ? tUI("consoleCopy.lookingUp") : tUI("consoleCopy.lookUpRefundableBill")}</Button>
      <Link className="py-2 text-sm underline" href="/admin/usage">{tUI("consoleCopy.viewUsageBills")}</Link>
    </form>
    {review && item && <div className="mt-3 space-y-2 break-words text-sm">
      <p>{tUI("consoleCopy.refundRecipient")}{person}</p>
      <p>{tUI("consoleCopy.model")}{item.model_id || "—"} {tUI("consoleCopy.originalCharge")}{money(item.amount_minor)}</p>
      <p>{tUI("consoleCopy.topUpCreditToReturn")}{money(item.wallet_minor)} {tUI("consoleCopy.originalPlanTimeLimitedCreditToRestore")}{money(item.entitlement_minor)}</p>
      <p>{tUI("consoleCopy.walletGiftCreditNotReturned")}{money(item.gift_minor)}{tUI("consoleCopy.restoredCreditRetainsItsOriginalExpiryExpiredCredit")}</p>
      {item.state === "reversed" ? <p role="status">{tUI("consoleCopy.thisBillHasAlreadyBeenRefundedNoFurther")}</p> : item.state === "committed" ? <Button variant="outline" onClick={() => { setError(""); setOpen(true); }}>{tUI("consoleCopy.reviewAndRefundUsageBill")}</Button> : <p role="status">{tUI("consoleCopy.thisBillCannotCurrentlyBeRefunded")}</p>}
    </div>}
    {error && !open && <p role="alert" className="mt-2 text-sm text-danger">{error}</p>}
    {message && <p role="status" className="mt-2 text-sm">{message}</p>}
    <ConfirmDialog open={open} onOpenChange={setOpen} title={tUI("consoleCopy.confirmUsageBillRefund")} confirmLabel={tUI("consoleCopy.confirmRefund")} error={error}
      description={item ? tUI("consoleCopy.requestReturnTopUpCreditAndRestoreOriginal", { value0: String(person), value1: String(item.request_id), value2: String(money(item.wallet_minor)), value3: String(money(item.entitlement_minor)), value4: String(money(item.gift_minor)) }) : ""} onConfirm={refund} />
  </section>;
}
