import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import ProfilePanel from "./profile-panel";

export default function ProfilePage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="profile" />
      <ProfilePanel />
    </div>
  );
}
