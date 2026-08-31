import { headers } from "next/headers";
import Link from "next/link";
import { Badge } from "@/components/ui/badge";
import { PublicSection } from "@/components/public-section";
import { loadSite } from "@/lib/site-content";
import { I18nPublicHero } from "@/components/i18n-page-hero";

export default async function AwesomePage() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const site = await loadSite(host);
  const apps = site.apps || [];

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-12 px-6 py-20">
      <I18nPublicHero id="awesome" primaryHref="/login" secondaryHref="/vibe-coding" />
      <PublicSection eyebrow="APPS" title={`${apps.length} 个应用`}>
        <ul className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
          {apps.map((app) => (
            <li key={app.slug} className="rounded-card border border-hairline bg-canvas-raised p-5">
              <div className="flex flex-wrap items-center gap-2">
                <Badge tone="brand">{app.kind}</Badge>
                {app.oss ? <span className="th-eyebrow text-success">OSS</span> : null}
              </div>
              <h3 className="mt-3 text-lg font-semibold">{app.name}</h3>
              <p className="mt-2 text-sm text-ink-secondary">{app.summary}</p>
              <p className="mt-3 text-[12px] text-ink-mute">{app.date}</p>
            </li>
          ))}
        </ul>
      </PublicSection>
      <p className="text-sm text-ink-mute">
        想列出你的工具？先走 <Link href="/docs">文档</Link> 换 base URL。
      </p>
    </main>
  );
}
