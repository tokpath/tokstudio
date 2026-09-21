import { ChannelHero } from "./channel-hero";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import { TaskLinks } from "@/components/console/task-links";
export default function ChannelConsole() {
  return <div className="flex flex-col gap-8"><I18nConsoleHeader id="channelHome" /><ChannelHero /><TaskLinks items={[
    {id:"channelUsers",href:"/channel/users"}, {id:"channelModels",href:"/channel/models"},
    {id:"channelPlans",href:"/channel/plans"}, {id:"channelPayments",href:"/channel/payments"},
    {id:"channelLedger",href:"/channel/ledger"}, {id:"channelPromos",href:"/channel/promos"},
    {id:"channelUsage",href:"/channel/usage"}, {id:"channelSettlements",href:"/channel/settlements"},
  ]} /></div>;
}
