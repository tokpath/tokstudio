"use client";

import { useTranslations } from "next-intl";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function ReferralPage() {
  const t = useTranslations("user");

  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="referral" />
      <EmptyLedger title={t("refComing")} detail={t("refComingDetail")} />
    </div>
  );
}
