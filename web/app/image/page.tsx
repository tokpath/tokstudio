import { headers } from "next/headers";
import Link from "next/link";
import { Button } from "@/components/ui/button";
import { getTranslations } from "next-intl/server";
import { loadCatalog, priceForModel } from "@/lib/catalog";
import { IconStamp } from "@/components/icon-stamp";
import { I18nPublicHero } from "@/components/i18n-page-hero";
import { Image as ImageIcon } from "lucide-react";
import { PublicMain } from "@/components/public-section";

export default async function ImagePage() {
  const t = await getTranslations("imageUi");
  const tCat = await getTranslations("catalog");
  const host = (await headers()).get("x-tokenhub-host") || "localhost";
  const models = await loadCatalog(host, { kind: "image" });
  const priceUnits = { perSec: tCat("perSec"), perImage: tCat("perImage") };

  return (
    <PublicMain>
      <I18nPublicHero id="image" primaryHref="/login" secondaryHref="/models" />
      <ul className="grid gap-4 md:grid-cols-2">
        {models.map((m) => (
          <li key={m.id}>
            <Link
              href={`/models/${m.id}`}
              className="block rounded-card border border-hairline bg-canvas-raised p-5 no-underline hover:bg-brand-soft/30"
            >
              <div className="flex items-start justify-between gap-3">
                <p className="font-semibold text-ink">{m.display_name}</p>
                <IconStamp icon={ImageIcon} size="sm" />
              </div>
              <p className="mt-1 font-mono text-[12px] text-ink-mute">{m.id}</p>
              <p className="mt-3 font-mono text-sm tabular-nums text-brand-emphasis">{priceForModel(m, priceUnits).primary}</p>
              {m.description ? <p className="mt-2 line-clamp-2 text-[13px] text-ink-secondary">{m.description}</p> : null}
            </Link>
          </li>
        ))}
      </ul>
      <Button asChild variant="outline" className="w-fit">
        <Link href="/docs">{t("docs")}</Link>
      </Button>
    </PublicMain>
  );
}
