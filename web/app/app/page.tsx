import ExamplesPanel from "./examples";
import KeysPanel from "./keys";
import MediaPanel from "./media";
import PlansPanel from "./plans";
import SettingsPanel from "./settings";
import UsagePanel from "./usage";
import WalletPanel from "./wallet";

export default function UserConsole() {
  return (
    <main className="mx-auto flex max-w-5xl flex-col gap-6">
      <header className="mb-2">
        <p className="text-sm uppercase tracking-[0.2em] text-ink-secondary">用户控制台</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-tight">我的账户</h1>
        <p className="mt-2 max-w-2xl text-ink-secondary">
          这里只展示你自己的余额、套餐、API Key、用量、账单和个人设置。渠道归属在注册时已经写死，页面上不会提供“切换渠道”入口。
        </p>
      </header>
      <section className="grid gap-3 md:grid-cols-2 lg:grid-cols-4" aria-label="总览">
        {[
          { t: "可用余额", d: "现金钱包可调用额度", href: "#wallet" },
          { t: "预授权占用", d: "HOLD 中的请求尚未结算", href: "#wallet" },
          { t: "API Key", d: "掩码前缀，轮换写审计", href: "#keys" },
          { t: "路由回单", d: "最近一次 attempt 可解释", href: "#usage" },
        ].map((card) => (
          <a
            key={card.t}
            href={card.href}
            className="rounded-card border border-hairline bg-canvas-raised p-4 no-underline hover:bg-brand-soft/40"
          >
            <p className="th-eyebrow text-ink-mute">{card.t}</p>
            <p className="mt-2 text-sm text-ink-secondary">{card.d}</p>
          </a>
        ))}
      </section>
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
    </main>
  );
}
