import ChannelPlans from "../plans-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function ChannelPlansPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelPlans" />
      <ChannelPlans />
    </div>
  );
}
