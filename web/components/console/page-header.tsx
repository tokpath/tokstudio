import type { ReactNode } from "react";

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
    <header className="mb-6 flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
      <div>
        {eyebrow ? <p className="th-eyebrow text-ink-mute">{eyebrow}</p> : null}
        <h1 className={`text-2xl font-semibold tracking-tight ${eyebrow ? "mt-2" : ""}`}>{title}</h1>
        {description ? <p className="mt-2 max-w-2xl text-sm text-ink-secondary">{description}</p> : null}
      </div>
      {actions ? <div className="flex flex-wrap gap-2">{actions}</div> : null}
    </header>
  );
}
