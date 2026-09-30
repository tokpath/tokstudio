"use client";

import ChannelUsage from "../usage-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import { useViewer } from "@/components/rbac/viewer-context";
import { OEMScope } from "../management-panels";

export default function ChannelUsagePage() {
  const viewer = useViewer();
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelUsage" />
      {viewer.channelType === "C" ? <OEMScope includeAll={false}>{(suffix) => <ChannelUsage channelID={new URLSearchParams(suffix).get("channel_id") ?? undefined} />}</OEMScope> : <ChannelUsage />}
    </div>
  );
}
