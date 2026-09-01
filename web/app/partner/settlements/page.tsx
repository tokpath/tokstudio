import { PartnerBoard } from "../partner-board";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function PartnerSettlementsPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="partnerSettlements" />
      <PartnerBoard section="settlements" />
    </div>
  );
}
