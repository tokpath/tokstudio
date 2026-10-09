"use client";
import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { useViewer } from "@/components/rbac/viewer-context";
import { canAccessChannelPortal } from "@/lib/rbac";
import { loginHref } from "@/lib/login-next";
import { ChannelCommissionRecords } from "./settlements-panel";

export function LegacyCommissionEntry({ entry }: { entry: "rules" | "commissions" | "settlements" }) {
  const viewer = useViewer(), router = useRouter(), t = useTranslations('commissionWorkflow');
  useEffect(() => {
    if (viewer.loading || viewer.error) return;
    const params = new URLSearchParams(window.location.search);
    if (!viewer.signedIn) { router.replace(loginHref(`${window.location.pathname}${window.location.search}`)); return; }
    if (!canAccessChannelPortal(viewer.roles)) return;
    if (viewer.channelType !== 'C' && entry !== 'rules') return;
    const path = entry === 'commissions' ? '/channel/ledger' : '/channel/commission';
    params.set('tab', entry === 'rules' ? 'rules' : entry === 'settlements' ? 'payout' : 'issuance');
    router.replace(`${path}?${params}`);
  }, [viewer.loading, viewer.error, viewer.signedIn, viewer.channelType, viewer.roles, entry, router]);
  if (viewer.error) return <p role="alert">{t('loadFailed')}</p>;
  if (viewer.loading) return <p role="status">{t('loading')}</p>;
  if (!viewer.signedIn || !canAccessChannelPortal(viewer.roles)) return <p role="alert">{t('forbidden')}</p>;
  if (viewer.channelType === 'B' && entry === 'commissions') return <ChannelCommissionRecords key={viewer.userId} />;
  if (viewer.channelType === 'B' && entry === 'settlements') return <ChannelCommissionRecords key={viewer.userId} settlements />;
  return <p role="status">{t('loading')}</p>;
}
