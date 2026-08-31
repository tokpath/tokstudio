import Link from "next/link";
import { Button } from "@/components/ui/button";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import { getTranslations } from "next-intl/server";
import KeysPanel from "../keys-panel";

export default async function KeysPage() {
  const t = await getTranslations("console.keys");
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader
        id="keys"
        actions={
          <Button asChild variant="outline">
            <Link href="/app/docs">{t("docsCta")}</Link>
          </Button>
        }
      />
      <KeysPanel />
    </div>
  );
}
