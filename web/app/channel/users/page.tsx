"use client";

import ChannelUsers from "../users-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import Link from "next/link";
import { useViewer } from "@/components/rbac/viewer-context";
import { OEMScope } from "../management-panels";

export default function ChannelUsersPage() {
  const viewer = useViewer();
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelUsers" actions={<Link href="/channel/keys" className="text-sm text-brand-emphasis underline">用户 API Key</Link>} />
      {viewer.channelType === "C" ? <OEMScope includeAll={false}>{(suffix) => <ChannelUsers managedChannelID={new URLSearchParams(suffix).get("channel_id") ?? undefined} />}</OEMScope> : <ChannelUsers />}
    </div>
  );
}
