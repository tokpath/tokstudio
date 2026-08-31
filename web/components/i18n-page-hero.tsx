import { getTranslations } from "next-intl/server";
import type { ReactNode } from "react";
import { ConsolePageHeader } from "@/components/console/page-header";
import { PublicPageHero } from "@/components/public-section";

/** 公共站页头走 messages.public.<id>。URL 留在调用方，不写进 JSON。 */
export async function I18nPublicHero({
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
  const t = await getTranslations(`public.${id}`);
  return (
    <PublicPageHero
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
export async function I18nConsoleHeader({ id, actions }: { id: string; actions?: ReactNode }) {
  const t = await getTranslations(`console.${id}`);
  return <ConsolePageHeader eyebrow={t("eyebrow")} title={t("title")} description={t("description")} actions={actions} />;
}
