import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { Button } from "@/components/ui/button";
import { PublicSection, StatStrip } from "@/components/public-section";
import { I18nPublicHero } from "@/components/i18n-page-hero";

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
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-12 px-6 py-20">
      <I18nPublicHero id="desktop" primaryHref="/vibe-coding" secondaryHref="/login" />

      <StatStrip
        items={[
          { label: t("statInstall"), value: t("statInstallValue"), hint: t("statInstallHint") },
          { label: t("statPath"), value: "Web", hint: t("statPathHint") },
          { label: t("statBill"), value: t("statBillValue"), hint: t("statBillHint") },
        ]}
      />

      <PublicSection eyebrow="TOOLS" title={t("toolsTitle")} description={t("toolsLead")}>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {tools.map((tool) => (
            <div key={tool.name} className="rounded-card border border-hairline bg-canvas-raised p-4">
              <p className="font-medium text-ink">{tool.name}</p>
              <p className="mt-1 font-mono text-[13px] text-ink-mute">{tool.hint}</p>
            </div>
          ))}
        </div>
      </PublicSection>

      <PublicSection eyebrow="CAPABILITY" title={t("capsTitle")}>
        <div className="grid gap-3 md:grid-cols-2">
          {caps.map((cap) => (
            <div key={cap.t} className="rounded-card border border-hairline bg-canvas-raised p-5">
              <h3 className="text-lg font-semibold">{cap.t}</h3>
              <p className="mt-2 text-sm text-ink-secondary">{cap.d}</p>
            </div>
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
    </main>
  );
}
