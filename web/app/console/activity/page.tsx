import { Suspense } from "react";
import { ActivityTable } from "./activity-table";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function ActivityPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="activity" />
      <Suspense>
        <ActivityTable />
      </Suspense>
    </div>
  );
}
