import { BucketReconcilePanel } from "@/components/bucket-reconcile-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";

export default function ChannelReconciliationPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelReconciliation" />
      <BucketReconcilePanel scope="channel" />
    </div>
  );
}
