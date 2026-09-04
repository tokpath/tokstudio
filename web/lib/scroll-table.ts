import { cn } from "@/lib/utils";

export type ScrollTableDensity = "admin" | "ledger";

/** 宽表默认钉住首列 + 末列；列数 < 3 时不钉。 */
export function shouldStickyEnds(columnCount: number, stickyEnds = true): boolean {
  return stickyEnds && columnCount >= 3;
}

export function stickyColumnClass(
  index: number,
  columnCount: number,
  opts?: { stickyEnds?: boolean; density?: ScrollTableDensity; header?: boolean },
): string {
  const density = opts?.density ?? "admin";
  const pad = density === "ledger" ? "px-4 py-3" : "px-3 py-2.5";
  const header = opts?.header ? "th-eyebrow text-ink-mute" : "text-ink";
  const pin = shouldStickyEnds(columnCount, opts?.stickyEnds);
  if (!pin) {
    return cn(pad, header, "whitespace-nowrap");
  }
  if (index === 0) {
    return cn(
      pad,
      header,
      "sticky left-0 z-[1] whitespace-nowrap bg-canvas-raised shadow-[4px_0_8px_-4px_rgba(20,20,20,0.12)] group-hover:bg-brand-soft/40",
      opts?.header && "z-[2]",
    );
  }
  if (index === columnCount - 1) {
    return cn(
      pad,
      header,
      "sticky right-0 z-[1] whitespace-nowrap bg-canvas-raised shadow-[-4px_0_8px_-4px_rgba(20,20,20,0.12)] group-hover:bg-brand-soft/40",
      opts?.header && "z-[2]",
    );
  }
  return cn(pad, header, "whitespace-nowrap");
}
