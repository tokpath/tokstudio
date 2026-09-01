import Link from "next/link";
import type { LucideIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

/** 公共站内容宽与段落节奏：区块之间多留空气，页内元素自己收紧。 */
export function PublicMain({
  children,
  className,
  width = "site",
}: {
  children: React.ReactNode;
  className?: string;
  width?: "site" | "prose";
}) {
  return (
    <main
      className={cn(
        "mx-auto flex w-full flex-col gap-16 px-6 py-16 sm:gap-20 sm:py-20",
        width === "prose" ? "max-w-[720px]" : "max-w-[1200px]",
        className,
      )}
    >
      {children}
    </main>
  );
}

/** 公共站区块壳：纸面、眉题、标题、说明。 */
export function PublicSection({
  eyebrow,
  title,
  description,
  action,
  children,
  id,
}: {
  eyebrow?: string;
  title: string;
  description?: string;
  action?: React.ReactNode;
  children?: React.ReactNode;
  id?: string;
}) {
  return (
    <section id={id} className="scroll-mt-24 flex flex-col gap-8">
      <div className="flex flex-wrap items-end justify-between gap-5">
        <div className="max-w-2xl">
          {eyebrow ? <p className="th-eyebrow text-ink-mute">{eyebrow}</p> : null}
          <h2 className={`text-2xl font-semibold tracking-tight ${eyebrow ? "mt-3" : ""}`}>{title}</h2>
          {description ? <p className="mt-3 max-w-xl text-base leading-relaxed text-ink-secondary">{description}</p> : null}
        </div>
        {action}
      </div>
      {children}
    </section>
  );
}

export function PublicPageHero({
  eyebrow,
  title,
  description,
  primaryHref,
  primaryLabel,
  secondaryHref,
  secondaryLabel,
}: {
  eyebrow: string;
  title: string;
  description: string;
  primaryHref: string;
  primaryLabel: string;
  secondaryHref?: string;
  secondaryLabel?: string;
  icon?: LucideIcon;
}) {
  return (
    <section className="flex max-w-3xl flex-col">
      <p className="th-eyebrow text-brand-emphasis">{eyebrow}</p>
      <h1 className="th-display-sm mt-4">{title}</h1>
      <p className="mt-5 max-w-xl text-base leading-relaxed text-ink-secondary">{description}</p>
      <div className="mt-8 flex flex-wrap items-center gap-3">
        <Button asChild>
          <Link href={primaryHref}>{primaryLabel}</Link>
        </Button>
        {secondaryHref && secondaryLabel ? (
          <Button asChild variant="outline">
            <Link href={secondaryHref}>{secondaryLabel}</Link>
          </Button>
        ) : null}
      </div>
    </section>
  );
}

export function StatStrip({
  items,
}: {
  items: { label: string; value: string; hint?: string; icon?: LucideIcon }[];
}) {
  return (
    <div className="flex flex-wrap gap-x-12 gap-y-8">
      {items.map((item) => (
        <div key={item.label} className="min-w-[8rem]">
          <p className="th-eyebrow text-ink-mute">{item.label}</p>
          <p className="mt-3 font-mono text-[32px] font-medium leading-none tabular-nums">{item.value}</p>
          {item.hint ? <p className="mt-3 max-w-[16rem] text-[13px] leading-relaxed text-ink-mute">{item.hint}</p> : null}
        </div>
      ))}
    </div>
  );
}
