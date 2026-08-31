import ChannelAttribution from "./attribution-panel";
import ChannelCommissions from "./commissions-panel";
import ChannelModels from "./models";
import ChannelPlans from "./plans-panel";
import ChannelPromos from "./promos-panel";
import ChannelSettlements from "./settlements-panel";
import ChannelUsage from "./usage-panel";
import ChannelUsers from "./users-panel";
import { ChannelHero } from "./channel-hero";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function ChannelConsole() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelHome" />
      <ChannelHero />
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
    </div>
  );
}
