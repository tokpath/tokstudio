import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import { ChannelPaymentsNav } from "./payments-nav";
import { PaymentLanesPanel } from "./lanes-panel";

export default function ChannelPaymentsPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelPayments" />
      <ChannelPaymentsNav />
      <PaymentLanesPanel />
    </div>
  );
}
