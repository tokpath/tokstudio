import ChannelKeys from "../keys-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function ChannelKeysPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelKeys" />
      <ChannelKeys />
    </div>
  );
}
