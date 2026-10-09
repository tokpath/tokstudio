"use client";
import ChannelSettlements from "../settlements-panel";
import { SettlementPanel } from "@/app/admin/commission/settlement-panel";
import { useViewer } from "@/components/rbac/viewer-context";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
export default function Page() {
  const viewer = useViewer();
  return <div className="space-y-6"><I18nConsoleHeader id="channelSettlements" />{viewer.channelType === "C" ? <SettlementPanel oem /> : <ChannelSettlements />}</div>;
}
