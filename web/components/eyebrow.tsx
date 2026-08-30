export function Eyebrow({
  children,
  className = "text-ink-mute",
}: {
  children: string;
  className?: string;
}) {
  return <p className={`th-eyebrow ${className}`}>{children}</p>;
}
