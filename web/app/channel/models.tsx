"use client";


import { networkUnavailable } from "@/lib/confirm";
import { useTranslations } from "next-intl";
import { canChannelAction } from "@/lib/rbac";
import { useState } from "react";
import { ConfirmButton } from "@/components/confirm-button";
import { Button } from "@/components/ui/button";
import { Card, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { useViewer } from "@/components/rbac/viewer-context";
import { ListResourceView } from "@/components/console/list-resource-view";
import { useListResource } from "@/hooks/use-list-resource";
import { apiBase } from "@/lib/api";
import { confirmHeaders, confirmNetworkUnavailable } from "@/lib/confirm";
import { fetchListItems } from "@/lib/list-resource";
import { millionDim, perTokenToPerMillion } from "@/lib/token-price";

type ChannelModel = { public_id: string; display_name?: string; vendor?: string; kind?: string; status?: string; enabled?: boolean; self_enabled?: boolean; parent_enabled?: boolean; effective_enabled?: boolean; customer_override?: Record<string, string> };

export default function ChannelModels() {
  const tUI = useTranslations();
  const confirmNetworkUnavailable = networkUnavailable(tUI);

  const [message, setMessage] = useState("");
  const [editingPrice, setEditingPrice] = useState("");
  const [priceInput, setPriceInput] = useState("");
  const [priceOutput, setPriceOutput] = useState("");
  const [priceUnit, setPriceUnit] = useState("");
  const viewer = useViewer();
  const list = useListResource<ChannelModel>({
    load: () => fetchListItems(`${apiBase}/channel/models`),
  });

  async function changeOwnStatus(item: ChannelModel, enabled: boolean): Promise<boolean> {
    try {
      const res = await fetch(`${apiBase}/channel/models`, {
        method: "PATCH", credentials: "include", headers: confirmHeaders,
        body: JSON.stringify({ public_id: item.public_id, enabled }),
      });
      const body = await res.json();
      setMessage(res.ok ? tUI("consoleCopy.completed", { value0: String(enabled ? tUI("consoleCopy.enable") : tUI("consoleCopy.unpublish")), value1: String(item.display_name || item.public_id) }) : body.error?.message || tUI("user.actFail"));
      if (res.ok) await list.reload();
      return res.ok;
    } catch {
      setMessage(confirmNetworkUnavailable);
      return false;
    }
  }

  function startPrice(item: ChannelModel) {
    const unit = item.kind === "image" ? "image_count" : item.kind === "video" ? "video_second" : "audio_second";
    setEditingPrice(item.public_id);
    setPriceInput(perTokenToPerMillion(item.customer_override?.input || ""));
    setPriceOutput(perTokenToPerMillion(item.customer_override?.output || ""));
    setPriceUnit(item.customer_override?.[unit] || "");
    setMessage("");
  }

  async function savePrice(item: ChannelModel): Promise<boolean> {
    const kind = item.kind || "text";
    const unit = kind === "image" ? "image_count" : kind === "video" ? "video_second" : "audio_second";
    const textPrice = kind === "text" || kind === "embedding";
    const valid = (value: string) => /^\d+(?:\.\d+)?$/.test(value.trim()) && Number(value) >= 0;
    if (textPrice ? ((priceInput || priceOutput) && (!valid(priceInput) || (kind === "text" && !valid(priceOutput)))) : (priceUnit && !valid(priceUnit))) {
      setMessage(tUI("consoleCopy.enterACompleteNonnegativeBrandCustomerPriceOr"));
      return false;
    }
    let prices: Record<string, string> = {};
    try {
      prices = textPrice ? (priceInput ? millionDim(priceInput, kind === "text" ? priceOutput : "", tUI) || {} : {}) : (priceUnit ? { [unit]: priceUnit.trim() } : {});
    } catch (error) {
      setMessage(error instanceof Error ? error.message : tUI("consoleCopy.invalidPriceFormat"));
      return false;
    }
    try {
      const response = await fetch(`${apiBase}/channel/model-prices`, {
        method: "PATCH", credentials: "include", headers: confirmHeaders,
        body: JSON.stringify({ public_id: item.public_id, customer_price: prices }),
      });
      const body = await response.json();
      setMessage(response.ok ? tUI("consoleCopy.brandCustomerPriceUpdatedFor", { value0: String(item.display_name || item.public_id) }) : body.error?.message || tUI("user.saveFail"));
      if (response.ok) {
        setEditingPrice("");
        await list.reload();
      }
      return response.ok;
    } catch {
      setMessage(confirmNetworkUnavailable);
      return false;
    }
  }

  return (
    <Card>
      <CardTitle className="mb-4 text-lg font-semibold tracking-tight">{tUI("channelNav.models")}</CardTitle>
      <p className="mb-3 text-sm text-ink-secondary">
        {tUI("consoleCopy.youCanUnpublishModelsForThisChannelChild")}{viewer.channelType === "C" ? tUI("consoleCopy.setBrandCustomerPricesHereChildChannelsInherit") : tUI("consoleCopy.customerPricesAreSetCentrallyByTheOwning")}
      </p>
      <Button variant="outline" onClick={() => void list.reload()}>
        {tUI("consoleCopy.refreshModels")}</Button>
      {message ? <p role="status" className="mt-2 text-sm text-ink-secondary">{message}</p> : null}
      <ListResourceView
        snapshot={list.snapshot}
        emptyTitle={tUI("consoleCopy.noGrantedModels")}
        emptyDetail={tUI("consoleCopy.thePlatformHasNotGrantedModelsToThis")}
        onRetry={() => void list.reload()}
      >
        <ul className="mt-4 grid gap-2 text-sm">
          {list.snapshot.items.map((item) => (
          <li key={item.public_id} className="flex flex-wrap justify-between gap-2 border-b border-hairline py-2">
              <span>
                {item.display_name || item.public_id} <span className="font-mono text-ink-secondary">{item.public_id}</span>
              </span>
            <span className="text-ink-secondary">
                {item.vendor} · {item.status !== "published" ? tUI("consoleCopy.unpublishedByPlatform") : !item.enabled ? tUI("consoleCopy.grantRevokedByParent") : item.parent_enabled === false ? tUI("consoleCopy.unpublishedByParent") : item.self_enabled === false ? tUI("consoleCopy.unpublishedForThisChannel") : tUI("entitlements.active")}
            </span>
            {item.enabled && canChannelAction("operations", viewer) ? <ConfirmButton
              size="sm" variant="outline"
              disabled={item.status !== "published" || (item.parent_enabled === false && item.self_enabled === false)}
              title={item.self_enabled === false ? tUI("consoleCopy.confirmModelEnabling") : tUI("consoleCopy.confirmModelUnpublication")}
              description={item.self_enabled === false ? tUI("consoleCopy.restoreUseForThisChannelAndGrantedChild") : tUI("consoleCopy.immediatelyDisableThisModelForThisChannelAnd")}
              onConfirm={() => changeOwnStatus(item, item.self_enabled === false)}
            >{item.self_enabled === false ? tUI("consoleCopy.enable") : tUI("consoleCopy.unpublish")}</ConfirmButton> : null}
            {viewer.channelType === "C" && item.enabled && canChannelAction("finance", viewer) ? <Button size="sm" variant="outline" onClick={() => startPrice(item)}>{tUI("consoleCopy.brandCustomerPrice")}</Button> : null}
            {editingPrice === item.public_id ? <div className="mt-2 grid w-full gap-2 rounded-control border border-hairline p-3">
              {item.kind === "image" || item.kind === "video" || item.kind === "audio" ? <label className="grid gap-1">{tUI("consoleCopy.customerPriceUsdUnit")}<Input aria-label={tUI("consoleCopy.brandCustomerUnitPrice")} value={priceUnit} onChange={(event) => setPriceUnit(event.target.value)} /></label> : <>
                <label className="grid gap-1">{tUI("consoleCopy.inputUsdMillion492")}<Input aria-label={tUI("consoleCopy.brandCustomerInputPrice")} value={priceInput} onChange={(event) => setPriceInput(event.target.value)} /></label>
                {item.kind !== "embedding" ? <label className="grid gap-1">{tUI("consoleCopy.outputUsdMillion494")}<Input aria-label={tUI("consoleCopy.brandCustomerOutputPrice")} value={priceOutput} onChange={(event) => setPriceOutput(event.target.value)} /></label> : null}
              </>}
              <p className="text-xs text-ink-secondary">{tUI("consoleCopy.leaveBlankToUseThePlatformPublicPrice")}</p>
              <div className="flex gap-2"><ConfirmButton size="sm" title={tUI("consoleCopy.confirmBrandCustomerPriceChanges")} description={tUI("consoleCopy.newRequestsFromThisBrandAndItsChild")} onConfirm={() => savePrice(item)}>{tUI("consoleCopy.savePrice499")}</ConfirmButton><Button size="sm" variant="outline" onClick={() => setEditingPrice("")}>{tUI("common.cancel")}</Button></div>
            </div> : null}
          </li>
          ))}
        </ul>
      </ListResourceView>
    </Card>
  );
}
