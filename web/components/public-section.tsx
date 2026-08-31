import Link from "next/link";
import { Button } from "@/components/ui/button";

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
    <section id={id} className="scroll-mt-24 flex flex-col gap-6">
      <div className="flex flex-wrap items-end justify-between gap-4">
        <div className="max-w-2xl">
          {eyebrow ? <p className="th-eyebrow text-ink-mute">{eyebrow}</p> : null}
          <h2 className="mt-2 text-2xl font-semibold tracking-tight">{title}</h2>
          {description ? <p className="mt-2 text-sm text-ink-secondary">{description}</p> : null}
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
}) {
  return (
    <section className="flex max-w-3xl flex-col gap-4">
      <p className="th-eyebrow text-brand-emphasis">{eyebrow}</p>
      <h1 className="text-[40px] font-semibold leading-tight tracking-tight">{title}</h1>
      <p className="text-base text-ink-secondary">{description}</p>
      <div className="flex flex-wrap gap-3">
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
  items: { label: string; value: string; hint?: string }[];
}) {
  return (
    <div className="grid gap-3 sm:grid-cols-3">
      {items.map((item) => (
        <div key={item.label} className="rounded-stamp border border-hairline bg-canvas-raised px-4 py-4">
          <p className="th-eyebrow text-ink-mute">{item.label}</p>
          <p className="mt-2 font-mono text-xl font-medium tabular-nums">{item.value}</p>
          {item.hint ? <p className="mt-1 text-[13px] text-ink-mute">{item.hint}</p> : null}
        </div>
      ))}
    </div>
  );
}
