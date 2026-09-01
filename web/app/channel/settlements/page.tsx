import ChannelSettlements from "../settlements-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function ChannelSettlementsPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelSettlements" />
      <ChannelSettlements />
    </div>
  );
}
