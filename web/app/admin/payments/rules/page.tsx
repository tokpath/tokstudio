"use client";
import { useTranslations } from "next-intl";
import { AdminShell } from "../../shell";
import { PaymentRulesPanel } from "@/app/channel/payments/rules-panel";
import { PaymentsNav } from "../payments-nav";
export default function Page() { const t=useTranslations("paymentTasks"); return <AdminShell><h1 className="text-2xl font-semibold">{t("rulesTitle")}</h1><PaymentsNav scope="admin" active="rules" /><PaymentRulesPanel scope="admin" /></AdminShell>; }
