"use client";
import { useTranslations } from "next-intl";
import { AdminShell } from "../../shell";
import { PaymentLanesPanel } from "@/app/channel/payments/lanes-panel";
import { PaymentsNav } from "../payments-nav";
export default function Page() { const t=useTranslations("paymentTasks"); return <AdminShell><h1 className="text-2xl font-semibold">{t("lanesTitle")}</h1><PaymentsNav scope="admin" active="lanes" /><PaymentLanesPanel scope="admin" /></AdminShell>; }
