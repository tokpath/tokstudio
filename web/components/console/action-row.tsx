import type { ReactNode } from "react";
import { cn } from "@/lib/utils";

/**
 * 控制台操作按钮行：优先横排。
 * shrink-0 + w-max 避免被旁侧长文案挤成竖列；仅在整行宽度不够时才换行。
 */
export function ActionRow({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("flex w-max max-w-full shrink-0 flex-row flex-wrap items-center gap-2", className)}>
      {children}
    </div>
  );
}

/** 说明文案 + 操作按钮：文案可收缩，按钮组不收缩、保持横排。 */
export function LeadActions({
  lead,
  actions,
  className,
}: {
  lead?: ReactNode;
  actions: ReactNode;
  className?: string;
}) {
  return (
    <div className={cn("mb-4 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between", className)}>
      {lead ? <div className="min-w-0 flex-1">{lead}</div> : null}
      <ActionRow className="sm:pt-0.5">{actions}</ActionRow>
    </div>
  );
}
