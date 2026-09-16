import { getTranslations } from "next-intl/server";

export async function CatalogLoadingBlock() {
  const tc = await getTranslations("common");
  return (
    <div
      className="mt-3 rounded-card border border-hairline bg-canvas-raised px-8 py-12"
      data-testid="list-resource"
      data-list-phase="loading"
      aria-busy="true"
      aria-live="polite"
    >
      <p className="text-sm font-medium text-ink">{tc("listLoading")}</p>
      <p className="mt-2 text-sm text-ink-mute">{tc("listLoadingDetail")}</p>
    </div>
  );
}
