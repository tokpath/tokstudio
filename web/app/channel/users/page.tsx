import ChannelUsers from "../users-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function ChannelUsersPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelUsers" />
      <ChannelUsers />
    </div>
  );
}
