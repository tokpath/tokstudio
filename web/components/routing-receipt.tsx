/** 可解释路由的视觉回单。公共站示例必须带 EXAMPLE，不写真实 Key 或成本。 */
export function RoutingReceipt({
  eyebrow = "ATTEMPT",
  lines,
  footnote,
}: {
  eyebrow?: string;
  lines: string[];
  footnote?: string;
}) {
  return (
    <aside className="rounded-card border border-hairline bg-canvas-raised p-6">
      <p className="th-eyebrow text-ink-mute">{eyebrow}</p>
      <ol className="mt-4 flex flex-col gap-2.5 font-mono text-[13px] leading-relaxed text-ink">
        {lines.map((line) => (
          <li key={line}>{line}</li>
        ))}
      </ol>
      {footnote ? <p className="mt-5 text-[13px] leading-relaxed text-success">{footnote}</p> : null}
    </aside>
  );
}
