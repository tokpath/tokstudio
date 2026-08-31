import ExamplesPanel from "./examples-panel";
import KeysPanel from "./keys-panel";
import MediaPanel from "./media-panel";
import PlansPanel from "./plans-panel";
import SettingsPanel from "./settings-panel";
import UsagePanel from "./usage-panel";
import WalletPanel from "./wallet-panel";
import { OverviewHero } from "@/components/console/overview-hero";
import { getTranslations } from "next-intl/server";

export default async function UserConsole() {
  const t = await getTranslations("overview");
  return (
    <div className="flex flex-col gap-6">
      <header className="mb-2">
        <p className="text-sm uppercase tracking-[0.2em] text-ink-secondary">{t("eyebrow")}</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-tight">{t("title")}</h1>
        <p className="mt-2 max-w-2xl text-ink-secondary">{t("lead")}</p>
      </header>
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
