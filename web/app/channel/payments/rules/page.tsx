import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import { ChannelPaymentsNav } from "../payments-nav";
import { PaymentRulesPanel } from "../rules-panel";
export default function Page() { return <div className="space-y-6"><I18nConsoleHeader id="channelPaymentRules" /><ChannelPaymentsNav active="rules" /><PaymentRulesPanel /></div>; }
