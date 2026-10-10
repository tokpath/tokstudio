"use client";
import { useTranslations } from "next-intl";
import AdminDashboard from "./dashboard";
export default function AdminConsole() {
 const t=useTranslations("workbench");
 return <div className="flex flex-col gap-5"><h1 className="text-2xl font-semibold">{t("title")}</h1><AdminDashboard/></div>;
}
