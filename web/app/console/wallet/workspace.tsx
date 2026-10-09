"use client";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import { walletTabHref } from "@/lib/wallet-context";
import WalletPanel from "../wallet-panel";
import PlansPanel from "../plans-panel";
import { WalletLedger } from "./wallet-ledger";
import { WalletOrders } from "./wallet-orders";
export function WalletWorkspace(){
 const t=useTranslations("walletExperience");const search=useSearchParams();
 const requested=search.get("tab");const tab=["plans","records"].includes(requested || "") ? requested : search.get("plan") ? "plans" : "recharge";
 return <div className="space-y-5"><I18nConsoleHeader id="wallet"/><nav aria-label={t("tabs")} className="flex flex-wrap gap-2">{["recharge","plans","records"].map(key=><Button key={key} asChild variant={tab===key ? "default" : "outline"}><Link href={walletTabHref(search.toString(),key)} aria-current={tab===key ? "page" : undefined}>{t(key)}</Link></Button>)}</nav><div hidden={tab!=="recharge"}><WalletPanel/></div><div hidden={tab!=="plans"}><PlansPanel/></div><div hidden={tab!=="records"} className="space-y-8"><WalletOrders/><WalletLedger/></div></div>;
}
