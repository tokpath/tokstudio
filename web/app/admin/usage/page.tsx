"use client";
import { UsageReport } from "@/components/usage-report";
import { useTranslations } from "next-intl";
export default function AdminUsagePage(){const t=useTranslations("usageWorkflow");return <div className="flex flex-col gap-5"><h1 className="text-2xl font-semibold">{t("title")}</h1><UsageReport surface="admin"/></div>}
