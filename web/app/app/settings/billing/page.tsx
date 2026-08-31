import { EmptyLedger } from "@/components/console/empty-ledger";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import { getTranslations } from "next-intl/server";

export default async function BillingSettingsPage() {
  const t = await getTranslations("settingsEmpty");
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="billing" />
      <EmptyLedger title={t("billingTitle")} detail={t("billingDetail")} />
    </div>
  );
}
