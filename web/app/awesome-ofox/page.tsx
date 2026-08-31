import { headers } from "next/headers";
import Link from "next/link";
import { Badge } from "@/components/ui/badge";
import { PublicPageHero, PublicSection } from "@/components/public-section";
import { loadSite } from "@/lib/site-content";

export default async function AwesomePage() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const site = await loadSite(host);
  const apps = site.apps || [];

  return (
    <main className="mx-auto flex w-full max-w-[1120px] flex-col gap-12 px-6 py-16">
      <PublicPageHero
        eyebrow="WORKS WITH"
        title="用公开价目做点有趣的事"
        description="对齐 ofox Works with 墙：展示已接入的应用。皮肤是纸面细线，不堆营销渐变。"
        primaryHref="/login"
        primaryLabel="开始接入"
        secondaryHref="/vibe-coding"
        secondaryLabel="编程工具"
      />
      <PublicSection eyebrow="APPS" title={`${apps.length} 个应用`}>
        <ul className="grid gap-3 md:grid-cols-2 lg:grid-cols-3">
          {apps.map((app) => (
            <li key={app.slug} className="rounded-stamp border border-hairline bg-canvas-raised p-5">
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
