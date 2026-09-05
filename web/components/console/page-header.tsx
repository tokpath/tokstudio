import type { ReactNode } from "react";
import { ActionRow } from "@/components/console/action-row";

export function ConsolePageHeader({
  eyebrow,
  title,
  description,
  actions,
}: {
  eyebrow?: string;
  title: string;
  description?: string;
  actions?: ReactNode;
}) {
  return (
    <header className="mb-8 flex flex-col gap-5 md:flex-row md:items-end md:justify-between">
      <div className="min-w-0 flex-1">
        {eyebrow ? <p className="th-eyebrow text-ink-mute">{eyebrow}</p> : null}
        <h1 className={`text-2xl font-semibold tracking-tight ${eyebrow ? "mt-3" : ""}`}>{title}</h1>
        {description ? <p className="mt-3 max-w-2xl text-sm leading-relaxed text-ink-secondary">{description}</p> : null}
      </div>
      {actions ? <ActionRow className="gap-3">{actions}</ActionRow> : null}
    </header>
  );
}
