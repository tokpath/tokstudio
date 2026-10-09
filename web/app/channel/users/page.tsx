"use client";

import ChannelUsers from "../users-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import Link from "next/link";

export default function ChannelUsersPage() {
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelUsers" actions={<Link href="/channel/keys" className="text-sm text-brand-emphasis underline">用户 API Key</Link>} />
      <ChannelUsers />
    </div>
  );
}
