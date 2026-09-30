"use client";

import { useTranslations } from "next-intl";
import type { ReactNode } from "react";
import { ConsolePageHeader } from "@/components/console/page-header";
import { PublicPageHero } from "@/components/public-section";
import { iconForPublicPage } from "@/lib/page-icons";
import { useViewer } from "@/components/rbac/viewer-context";

const oemPageKeys: Record<string, string> = {
  channelHome: "overview", channelUsers: "users", channelPlans: "plans",
  channelPayments: "payments", channelPaymentOrders: "payments", channelPaymentRules: "payments",
  channelLedger: "billing", channelUsage: "usage", channelReconciliation: "reconciliation",
  channelPromos: "promos",
};

/** 公共站页头走 messages.public.<id>。URL 留在调用方，不写进 JSON。 */
export function I18nPublicHero({
  id,
  primaryHref,
  secondaryHref,
  title,
  description,
}: {
  id: string;
  primaryHref: string;
  secondaryHref?: string;
  title?: string;
  description?: string;
}) {
  const t = useTranslations(`public.${id}`);
  return (
    <PublicPageHero
      icon={iconForPublicPage(id)}
      eyebrow={t("eyebrow")}
      title={title ?? t("title")}
      description={description ?? t("description")}
      primaryHref={primaryHref}
      primaryLabel={t("primary")}
      secondaryHref={secondaryHref}
      secondaryLabel={secondaryHref ? t("secondary") : undefined}
    />
  );
}

/** 控制台页头走 messages.console.<id>。 */
export function I18nConsoleHeader({ id, actions }: { id: string; actions?: ReactNode }) {
  const t = useTranslations(`console.${id}`);
  const ta = useTranslations("admin");
  const to = useTranslations("oem");
  const viewer = useViewer();
  const isOEM = viewer.channelType === "C" && Boolean(oemPageKeys[id]);
  return <ConsolePageHeader eyebrow={isOEM ? "OEM" : t("eyebrow")} title={isOEM ? ta(oemPageKeys[id]) : t("title")} description={isOEM ? to("scope") : t("description")} actions={actions} />;
}
