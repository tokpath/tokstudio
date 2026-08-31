import ChannelAttribution from "./attribution";
import ChannelCommissions from "./commissions";
import ChannelModels from "./models";
import ChannelPlans from "./plans";
import ChannelPromos from "./promos";
import ChannelSettlements from "./settlements";
import ChannelUsage from "./usage";
import ChannelUsers from "./users";

export default function ChannelConsole() {
  return (
    <main className="mx-auto flex max-w-5xl flex-col gap-6">
      <header>
        <p className="text-sm uppercase tracking-[0.2em] text-ink-secondary">渠道控制台</p>
        <h1 className="mt-2 text-3xl font-semibold tracking-tight">本渠道数据</h1>
        <p className="mt-2 max-w-2xl text-ink-secondary">
          渠道管理员只能看到自己渠道的用户、套餐、推广链接、归因和用量。模型资源只能使用平台已授权的目录，不能自己添加提供商和模型。后端会再校验 scope，前端隐藏菜单不是安全边界。
        </p>
      </header>
      <div id="users">
        <ChannelUsers />
      </div>
      <div id="models">
        <ChannelModels />
      </div>
      <div id="plans">
        <ChannelPlans />
      </div>
      <div id="promos">
        <ChannelPromos />
      </div>
      <div id="attribution">
        <ChannelAttribution />
      </div>
      <div id="usage">
        <ChannelUsage />
      </div>
      <div id="settlements">
        <ChannelSettlements />
      </div>
      <div id="commissions">
        <ChannelCommissions />
      </div>
    </main>
  );
}
