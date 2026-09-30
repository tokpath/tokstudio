"use client";

import { BucketReconcilePanel } from "@/components/bucket-reconcile-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import { useViewer } from "@/components/rbac/viewer-context";
import { OEMScope } from "../management-panels";

export default function ChannelReconciliationPage() {
  const viewer = useViewer();
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelReconciliation" />
      {viewer.channelType === "C" ? <OEMScope includeAll={false}>{(suffix) => <BucketReconcilePanel scope="channel" channelID={new URLSearchParams(suffix).get("channel_id") ?? undefined} />}</OEMScope> : <BucketReconcilePanel scope="channel" />}
    </div>
  );
}
