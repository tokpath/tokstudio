import ChannelLedger from "../ledger-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import Link from "next/link";

export default function ChannelLedgerPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelLedger" actions={<Link href="/channel/commissions" className="text-sm text-brand-emphasis underline">用户额度发放</Link>} />
      <ChannelLedger />
    </div>
  );
}
