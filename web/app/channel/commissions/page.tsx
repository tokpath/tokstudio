import ChannelCommissions from "../commissions-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function ChannelCommissionsPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelCommissions" />
      <ChannelCommissions />
    </div>
  );
}
