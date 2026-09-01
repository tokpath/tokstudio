import type { ReactNode } from "react";

/** 控制台空账本：ofox「暂无数据」密度，皮肤走 DESIGN.md 纸面卡片。 */
export function EmptyLedger({
  title,
  detail,
  action,
}: {
  title: string;
  detail: string;
  action?: ReactNode;
}) {
  return (
    <div className="rounded-card border border-dashed border-hairline bg-canvas-raised px-6 py-12">
      <p className="text-sm font-medium text-ink">{title}</p>
      <p className="mt-2 max-w-xl text-sm text-ink-mute">{detail}</p>
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  );
}
