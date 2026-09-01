import Link from "next/link";
import type { LucideIcon } from "lucide-react";
import { IconStamp } from "@/components/icon-stamp";
import { cn } from "@/lib/utils";

/** 带章的内容卡：首页 why/工具、企业能力、信任摘要等共用。 */
export function FeatureCard({
  icon,
  title,
  description,
  meta,
  href,
  className,
}: {
  icon: LucideIcon;
  title: string;
  description?: string;
  meta?: string;
  href?: string;
  className?: string;
}) {
  const inner = (
    <>
      <IconStamp icon={icon} />
      <p className="mt-3 font-semibold text-ink">{title}</p>
      {description ? <p className="mt-2 text-sm leading-relaxed text-ink-secondary">{description}</p> : null}
      {meta ? <p className="mt-1 font-mono text-[12px] text-ink-mute">{meta}</p> : null}
    </>
  );
  const cls = cn(
    "rounded-card border border-hairline bg-canvas-raised p-5",
    href && "block no-underline transition-colors hover:bg-brand-soft/30",
    className,
  );
  if (href) {
    return (
      <Link href={href} className={cls}>
        {inner}
      </Link>
    );
  }
  return <div className={cls}>{inner}</div>;
}

/** 控制台 / 管理台数字卡：章在右上，金额等宽。 */
export function MetricCard({
  icon,
  label,
  value,
  hint,
  href,
  compact,
}: {
  icon: LucideIcon;
  label: string;
  value: string;
  hint?: string;
  href?: string;
  compact?: boolean;
}) {
  const inner = (
    <>
      <div className="flex items-start justify-between gap-3">
        <p className="th-eyebrow text-ink-mute">{label}</p>
        <IconStamp icon={icon} size="sm" />
      </div>
      <p
        className={`mt-2 font-mono font-medium tracking-tight text-ink ${
          compact ? "text-sm leading-snug" : "text-[28px] leading-none tabular-nums"
        }`}
      >
        {value}
      </p>
      {hint ? <p className="mt-2 text-sm text-ink-secondary">{hint}</p> : null}
    </>
  );
  const cls = cn(
    "rounded-card border border-hairline bg-canvas-raised p-4",
    href && "block no-underline hover:bg-brand-soft/40",
  );
  if (href) {
    return (
      <Link href={href} className={cls}>
        {inner}
      </Link>
    );
  }
  return <div className={cls}>{inner}</div>;
}
