import { Button } from "@/components/ui/button";
import { EmptyLedger } from "@/components/console/empty-ledger";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import { getTranslations } from "next-intl/server";

export default async function MembersSettingsPage() {
  const t = await getTranslations("settingsEmpty");
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="members" />
      <EmptyLedger
        title={t("membersTitle")}
        detail={t("membersDetail")}
        action={
          <Button type="button" variant="outline" disabled>
            {t("invite")}
          </Button>
        }
      />
    </div>
  );
}
