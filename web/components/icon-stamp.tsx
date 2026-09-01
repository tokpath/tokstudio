import type { LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

const BOX = {
  sm: "size-6",
  md: "size-8",
  lg: "size-10",
} as const;

const GLYPH = {
  sm: "size-3.5",
  md: "size-4",
  lg: "size-5",
} as const;

/** DESIGN.md：20–24px 几何章，圆角 6px。卡片、页头、空状态共用。 */
export function IconStamp({
  icon: Icon,
  size = "md",
  className,
}: {
  icon: LucideIcon;
  size?: keyof typeof BOX;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center justify-center rounded-[6px] bg-brand-soft text-brand-emphasis",
        BOX[size],
        className,
      )}
      aria-hidden
    >
      <Icon className={GLYPH[size]} strokeWidth={1.75} />
    </span>
  );
}
