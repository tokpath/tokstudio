import { headers } from "next/headers";
import Link from "next/link";
import { PublicPageHero } from "@/components/public-section";
import { Button } from "@/components/ui/button";
import { inferKind, loadCatalog, priceForModel } from "@/lib/catalog";

export default async function ImagePage() {
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const models = (await loadCatalog(host)).filter((m) => inferKind(m) === "image");

  return (
    <main className="mx-auto flex w-full max-w-[1200px] flex-col gap-10 px-6 py-20">
      <PublicPageHero
        eyebrow="IMAGE"
        title="图像模型"
        description={`${models.length} 个图像模型。价目驱动，不堆摄影英雄区。`}
        primaryHref="/login"
        primaryLabel="获取 API Key"
        secondaryHref="/models"
        secondaryLabel="全部模型"
      />
      <ul className="grid gap-3 md:grid-cols-2">
        {models.map((m) => (
          <li key={m.id}>
            <Link
              href={`/models/${m.id}`}
              className="block rounded-card border border-hairline bg-canvas-raised p-5 no-underline hover:bg-brand-soft/30"
            >
              <p className="font-semibold text-ink">{m.display_name}</p>
              <p className="mt-1 font-mono text-[12px] text-ink-mute">{m.id}</p>
              <p className="mt-3 font-mono text-sm tabular-nums text-brand-emphasis">{priceForModel(m).primary}</p>
              {m.description ? <p className="mt-2 line-clamp-2 text-[13px] text-ink-secondary">{m.description}</p> : null}
            </Link>
          </li>
        ))}
      </ul>
      <Button asChild variant="outline" className="w-fit">
        <Link href="/docs">查看接入文档</Link>
      </Button>
    </main>
  );
}
