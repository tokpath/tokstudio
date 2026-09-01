import ChannelPromos from "../promos-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function ChannelPromosPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelPromos" />
      <ChannelPromos />
    </div>
  );
}
