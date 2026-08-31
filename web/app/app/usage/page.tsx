import UsagePanel from "../usage-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function UsagePage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="usage" />
      <UsagePanel />
    </div>
  );
}
