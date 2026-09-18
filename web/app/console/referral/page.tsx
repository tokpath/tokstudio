"use client";

import { ReferralPanel } from "@/components/console/referral-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function ReferralPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="referral" />
      <ReferralPanel />
    </div>
  );
}
