import PlansPanel from "../plans-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function PlansPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="plans" />
      <PlansPanel />
    </div>
  );
}
