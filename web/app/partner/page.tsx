import { ReferralPanel } from "@/components/console/referral-panel";
import { PartnerBoard } from "./partner-board";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function PartnerConsole() {
  return (
    <div className="flex flex-col gap-8">
      <I18nConsoleHeader id="partnerHome" />
      <ReferralPanel />
      <PartnerBoard section="all" />
    </div>
  );
}
