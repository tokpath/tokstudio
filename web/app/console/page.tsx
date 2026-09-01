import ExamplesPanel from "./examples-panel";
import KeysPanel from "./keys-panel";
import MediaPanel from "./media-panel";
import PlansPanel from "./plans-panel";
import SettingsPanel from "./settings-panel";
import UsagePanel from "./usage-panel";
import WalletPanel from "./wallet-panel";
import { OverviewHero } from "@/components/console/overview-hero";
import { ConsolePageHeader } from "@/components/console/page-header";
import { getTranslations } from "next-intl/server";

export default async function UserConsole() {
  const t = await getTranslations("overview");
  return (
    <div className="flex flex-col gap-8">
      <ConsolePageHeader eyebrow={t("eyebrow")} title={t("title")} description={t("lead")} />
      <OverviewHero />
      <div id="wallet">
        <WalletPanel />
      </div>
      <div id="plans">
        <PlansPanel />
      </div>
      <div id="keys">
        <KeysPanel />
      </div>
      <div id="examples">
        <ExamplesPanel />
      </div>
      <div id="usage">
        <UsagePanel />
      </div>
      <div id="media">
        <MediaPanel />
      </div>
      <div id="settings">
        <SettingsPanel />
      </div>
    </div>
  );
}
