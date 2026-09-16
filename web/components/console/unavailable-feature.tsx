import { Ban } from "lucide-react";
import { getTranslations } from "next-intl/server";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

/** 未上线能力：入口已标明，页内不再假装有禁用按钮可点。 */
export async function UnavailableFeaturePage({ id }: { id: string }) {
  const t = await getTranslations("chrome");
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id={id} />
      <EmptyLedger title={t("unavailableTitle")} detail={t("unavailableDetail")} icon={Ban} />
    </div>
  );
}
