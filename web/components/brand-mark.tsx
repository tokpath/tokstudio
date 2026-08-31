/** 6px 钴方：小标记不能套按钮的 10px，否则会变成圆点。 */
export function BrandMark({ className = "" }: { className?: string }) {
  return (
    <span
      className={`inline-block size-6 shrink-0 rounded-[6px] bg-brand ${className}`}
      aria-hidden
    />
  );
}
