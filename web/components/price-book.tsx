import { CatalogFrame } from "@/components/catalog-frame";
import { EmptyState } from "@/components/empty-state";

export const PRICE_BOOK_COLUMNS = [
  "public_model_id",
  "厂商",
  "输入",
  "输出",
  "媒体",
  "能力",
  "状态",
];

export function PriceBookEmpty({ title, detail }: { title: string; detail: string }) {
  return (
    <CatalogFrame columns={PRICE_BOOK_COLUMNS}>
      <EmptyState title={title} detail={detail} />
    </CatalogFrame>
  );
}
