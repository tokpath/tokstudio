"use client";
import { useTranslations } from "next-intl";
import { Suspense } from "react";
import { useSearchParams } from "next/navigation";
import { ProfessionalCustomerDetail } from "@/components/professional-customer-detail";
function Detail() { const t = useTranslations("customerExperience"); const search = useSearchParams(); const role = search.get("role_id"); return role ? <ProfessionalCustomerDetail id={role} surface="admin"/> : <p role="alert">{t("selectAValidReferralCustomer")}</p>; }
export default function Page() { const t = useTranslations("customerExperience"); return <Suspense><Detail /></Suspense>; }
