"use client";
import { useTranslations } from "next-intl";
import { useState } from "react";
import { ConfirmDialog } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { apiBase } from "@/lib/api";
import { confirmHeaders } from "@/lib/confirm";
export function RecalcPanel() {
  const tUI = useTranslations();
 const [reference,setReference]=useState("");const [operation,setOperation]=useState("");const [open,setOpen]=useState(false);const [error,setError]=useState("");const [message,setMessage]=useState("");
 async function submit(){setError("");try{const r=await fetch(`${apiBase}/admin/commissions/recalc`,{method:"POST",credentials:"include",headers:confirmHeaders,body:JSON.stringify({usage_event_id:operation})});const body=await r.json();if(!r.ok){setError(body.error?.message||tUI("consoleCopy.theReviewIsIncompletePleaseRetry"));return false;}setMessage(body.item?.changed?tUI("consoleCopy.frozenCommissionsCorrectedUsingTheOriginalCalculationSnapshot"):body.item?.status==="reversed"?tUI("consoleCopy.thisUsageHasBeenReversedCommissionsWillNot"):body.item?.status==="no_active_commission"?tUI("consoleCopy.noValidCommissionRequiresReviewForThisUsage"):tUI("consoleCopy.reviewCompleteTheOriginalCommissionMatchesTheCalculation"));return true;}catch{setError(tUI("consoleCopy.theConnectionWasInterruptedAndTheResultIs"));return false;}}
 return <section className="rounded-card border border-hairline bg-canvas-raised p-6" aria-label={tUI("consoleCopy.commissionReviewAndRecalculation")}><h2 className="text-lg font-semibold">{tUI("consoleCopy.commissionReviewAndRecalculation")}</h2><p className="my-3 text-sm text-ink-secondary">{tUI("consoleCopy.reviewUsingSnapshotsOfTheOriginalPriceCommission")}</p><label className="block text-sm">{tUI("consoleCopy.usageRequestIdOrUsageId")}<Input value={reference} disabled={open} onChange={e=>setReference(e.target.value)} placeholder={tUI("consoleCopy.copyTheCorrespondingIdFromUsageBills")} /></label><Button className="mt-3" disabled={!reference.trim()||open} onClick={()=>{setOperation(reference.trim());setError("");setOpen(true);}}>{tUI("consoleCopy.reviewCommission")}</Button>{message&&<p role="status" className="mt-3 text-sm">{message}</p>}<ConfirmDialog open={open} onOpenChange={setOpen} title={tUI("consoleCopy.confirmCommissionReview")} description={tUI("consoleCopy.reviewUsageCurrentPolicyWillNotReplaceHistory", { value0: String(operation) })} error={error} onConfirm={submit}/></section>;
}
