import ChannelAttribution from "./attribution-panel";
import ChannelCommissions from "./commissions-panel";
import ChannelPlans from "./plans-panel";
import ChannelPromos from "./promos-panel";
import ChannelSettlements from "./settlements-panel";
import ChannelUsage from "./usage-panel";
import ChannelUsers from "./users-panel";
import { ChannelHero } from "./channel-hero";

export default function ChannelConsole() {
  return (
    <div className="flex flex-col gap-6">
      <header>
        <p className="th-eyebrow text-ink-mute">CHANNEL</p>
        <h1 className="mt-2 text-2xl font-semibold tracking-tight">本渠道数据</h1>
        <p className="mt-2 max-w-2xl text-ink-secondary">
          渠道管理员只能看到自己渠道的用户、套餐、推广链接、归因和用量。后端会再校验 scope，前端隐藏菜单不是安全边界。
        </p>
      </header>
      <ChannelHero />
      <div id="users">
        <ChannelUsers />
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
    </div>
  );
}
