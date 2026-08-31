import ExamplesPanel from "./examples-panel";
import KeysPanel from "./keys-panel";
import MediaPanel from "./media-panel";
import PlansPanel from "./plans-panel";
import SettingsPanel from "./settings-panel";
import UsagePanel from "./usage-panel";
import WalletPanel from "./wallet-panel";
import { OverviewHero } from "@/components/console/overview-hero";

export default function UserConsole() {
  return (
    <div className="flex flex-col gap-6">
      <header className="mb-2">
        <p className="text-sm uppercase tracking-[0.2em] text-ink-secondary">用户控制台</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-tight">我的账户</h1>
        <p className="mt-2 max-w-2xl text-ink-secondary">
          欢迎回来。这里只展示你自己的余额、套餐、API Key、用量、账单和个人设置。渠道归属在注册时已经写死，页面上不会提供“切换渠道”入口。
        </p>
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
