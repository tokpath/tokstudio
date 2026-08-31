import { PartnerBoard } from "../partner-board";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function PartnerCommissionsPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="partnerCommissions" />
      <PartnerBoard section="commissions" />
    </div>
  );
}
