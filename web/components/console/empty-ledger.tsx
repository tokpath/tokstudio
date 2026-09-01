import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { Inbox } from "lucide-react";
import { IconStamp } from "@/components/icon-stamp";

/** 控制台空账本：ofox「暂无数据」密度，皮肤走 DESIGN.md 纸面卡片。 */
export function EmptyLedger({
  title,
  detail,
  action,
  icon = Inbox,
}: {
  title: string;
  detail: string;
  action?: ReactNode;
  icon?: LucideIcon;
}) {
  return (
    <div className="rounded-card border border-dashed border-hairline bg-canvas-raised px-8 py-12">
      <IconStamp icon={icon} />
      <p className="mt-4 text-sm font-medium text-ink">{title}</p>
      <p className="mt-2 max-w-xl text-sm leading-relaxed text-ink-mute">{detail}</p>
      {action ? <div className="mt-5">{action}</div> : null}
    </div>
  );
}
