import ChannelAttribution from "../attribution-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function ChannelAttributionPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelAttribution" />
      <ChannelAttribution />
    </div>
  );
}
