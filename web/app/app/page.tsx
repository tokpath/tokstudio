import KeysPanel from "./keys";
import MediaPanel from "./media";
import PlansPanel from "./plans";
import UsagePanel from "./usage";
import WalletPanel from "./wallet";

export default function UserConsole() {
  return (
    <main className="mx-auto flex max-w-5xl flex-col gap-4 px-6 py-12">
      <p className="text-sm uppercase tracking-[0.2em] text-slate-400">用户控制台</p>
      <h1 className="text-3xl font-semibold">我的账户</h1>
      <p className="text-slate-300">
        这里只展示你自己的余额、套餐、API Key、用量和账单。渠道归属在注册时已经写死，页面上不会提供“切换渠道”入口。
      </p>
      <WalletPanel />
      <PlansPanel />
      <KeysPanel />
      <UsagePanel />
      <MediaPanel />
    </main>
  );
}
