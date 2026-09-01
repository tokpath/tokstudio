import ChannelModels from "../models";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function ChannelModelsPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelModels" />
      <ChannelModels />
    </div>
  );
}
