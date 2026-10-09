"use client";
import { useTranslations } from "next-intl";
import { AdminShell } from "../shell";
import { CommissionWorkspace } from "./workspace";
export default function AdminCommissionPage() { const t=useTranslations("admin"); return <AdminShell><h1 className="text-2xl font-semibold">{t("commission")}</h1><CommissionWorkspace /></AdminShell>; }
