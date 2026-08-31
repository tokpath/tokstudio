import SettingsPanel from "../settings-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function AccountSettingsPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="settings" />
      <SettingsPanel />
    </div>
  );
}
