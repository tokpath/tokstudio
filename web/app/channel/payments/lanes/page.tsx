import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import { ChannelPaymentsNav } from "../payments-nav";
import { PaymentLanesPanel } from "../lanes-panel";
export default function Page() { return <div className="space-y-6"><I18nConsoleHeader id="channelPayments" /><ChannelPaymentsNav active="lanes" /><PaymentLanesPanel /></div>; }
