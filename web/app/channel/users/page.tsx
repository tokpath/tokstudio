"use client";

import { useTranslations } from "next-intl";
import ChannelUsers from "../users-panel";
import { I18nConsoleHeader } from "@/components/i18n-page-hero";
import Link from "next/link";
import { useViewer } from "@/components/rbac/viewer-context";
import { canViewChannelHref } from "@/lib/rbac";

export default function ChannelUsersPage() {
  const tUI = useTranslations();
  const viewer = useViewer();
  return (
    <div className="flex flex-col gap-6">
      <I18nConsoleHeader id="channelUsers" actions={canViewChannelHref("/channel/keys", viewer) ? <Link href="/channel/keys" className="text-sm text-brand-emphasis underline">{tUI("consoleCopy.userApiKeys")}</Link> : undefined} />
      <ChannelUsers />
    </div>
  );
}
