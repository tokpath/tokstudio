export function EmptyState({ title, detail }: { title: string; detail: string }) {
  return (
    <div className="flex flex-col items-start gap-2 px-12 py-12">
      <p className="text-sm text-ink">{title}</p>
      <p className="text-sm text-ink-mute">{detail}</p>
    </div>
  );
}
