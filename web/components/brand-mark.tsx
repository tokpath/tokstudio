/** 10px 钴方：DESIGN.md 允许的几何章，不是发光 AI 标，也不是狐狸。 */
export function BrandMark({ className = "" }: { className?: string }) {
  return (
    <span
      className={`inline-block size-5 shrink-0 rounded-stamp bg-brand ${className}`}
      aria-hidden
    />
  );
}
