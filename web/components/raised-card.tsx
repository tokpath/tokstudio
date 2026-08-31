import type { ReactNode } from "react";
import { Eyebrow } from "@/components/eyebrow";

export function RaisedCard({
  eyebrow,
  title,
  children,
}: {
  eyebrow: string;
  title: string;
  children: ReactNode;
}) {
  return (
    <section className="rounded-card border border-hairline bg-canvas-raised p-6">
      <Eyebrow>{eyebrow}</Eyebrow>
      <h2 className="mt-2 text-lg font-semibold">{title}</h2>
      <div className="mt-3 text-sm text-ink-mute">{children}</div>
    </section>
  );
}
