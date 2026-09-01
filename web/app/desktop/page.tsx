import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { Button } from "@/components/ui/button";
import { PublicSection, StatStrip, PublicMain } from "@/components/public-section";
import { FeatureCard } from "@/components/feature-card";
import { I18nPublicHero } from "@/components/i18n-page-hero";
import { DESKTOP_CAP_ICONS, iconForTool } from "@/lib/page-icons";
import { Monitor, Receipt, Wallet } from "lucide-react";

export default async function DesktopPage() {
  const t = await getTranslations("desktopUi");
  const tools = [
    { name: "Claude Code", hint: "ANTHROPIC_BASE_URL" },
    { name: "Codex", hint: "~/.codex/config.toml" },
    { name: "Gemini CLI", hint: t("hintGemini") },
    { name: "OpenCode", hint: t("hintOpencode") },
    { name: "Cline", hint: t("hintCline") },
    { name: "OpenClaw", hint: t("hintOpenclaw") },
  ];
  const caps = [0, 1, 2, 3].map((i) => ({ t: t(`cap${i}t`), d: t(`cap${i}d`) }));

  return (
    <PublicMain>
      <I18nPublicHero id="desktop" primaryHref="/vibe-coding" secondaryHref="/login" />

      <StatStrip
        items={[
          { label: t("statInstall"), value: t("statInstallValue"), hint: t("statInstallHint"), icon: Monitor },
          { label: t("statPath"), value: "Web", hint: t("statPathHint"), icon: Wallet },
          { label: t("statBill"), value: t("statBillValue"), hint: t("statBillHint"), icon: Receipt },
        ]}
      />

      <PublicSection eyebrow="TOOLS" title={t("toolsTitle")} description={t("toolsLead")}>
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {tools.map((tool) => (
            <FeatureCard key={tool.name} icon={iconForTool(tool.name)} title={tool.name} meta={tool.hint} />
          ))}
        </div>
      </PublicSection>

      <PublicSection eyebrow="CAPABILITY" title={t("capsTitle")}>
        <div className="grid gap-4 md:grid-cols-2">
          {caps.map((cap, i) => (
            <FeatureCard key={cap.t} icon={DESKTOP_CAP_ICONS[i]} title={cap.t} description={cap.d} />
          ))}
        </div>
      </PublicSection>

      <PublicSection eyebrow="STATUS" title={t("statusTitle")}>
        <p className="text-sm text-ink-secondary">{t("statusBody")}</p>
        <div className="mt-4 flex flex-wrap gap-3">
          <Button asChild>
            <Link href="/quickstart">{t("webQs")}</Link>
          </Button>
          <Button asChild variant="outline">
            <Link href="/vibe-coding">Vibe Coding</Link>
          </Button>
        </div>
      </PublicSection>
    </PublicMain>
  );
}
