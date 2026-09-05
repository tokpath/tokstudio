import { OverviewHero } from "@/components/console/overview-hero";
import { ConsolePageHeader } from "@/components/console/page-header";
import { getTranslations } from "next-intl/server";

/** 总览：仅个人维度统计 + 快捷入口。业务面板各走独立路由（docs/14）。 */
export default async function UserConsole() {
  const t = await getTranslations("overview");
  return (
    <div className="flex flex-col gap-8">
      <ConsolePageHeader eyebrow={t("eyebrow")} title={t("title")} description={t("lead")} />
      <OverviewHero />
    </div>
  );
}
