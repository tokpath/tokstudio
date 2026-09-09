import { BucketReconcilePanel } from "@/components/bucket-reconcile-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function UserReconciliationPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="reconciliation" />
      <BucketReconcilePanel scope="user" />
    </div>
  );
}
