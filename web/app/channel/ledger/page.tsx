import ChannelLedger from "../ledger-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function ChannelLedgerPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelLedger" />
      <ChannelLedger />
    </div>
  );
}
