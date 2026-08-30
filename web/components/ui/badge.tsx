import { cn } from "@/lib/utils";

export function Badge({
  className,
  tone = "neutral",
  ...props
}: React.HTMLAttributes<HTMLSpanElement> & { tone?: "neutral" | "brand" | "success" | "warn" }) {
  const tones = {
    neutral: "border-white/10 bg-white/5 text-slate-300",
    brand: "border-[color-mix(in_srgb,var(--brand-primary)_40%,transparent)] bg-[color-mix(in_srgb,var(--brand-primary)_12%,transparent)] text-[var(--brand-primary)]",
    success: "border-emerald-400/20 bg-emerald-400/10 text-emerald-200",
    warn: "border-amber-400/20 bg-amber-400/10 text-amber-200",
  };
  return (
    <span
      className={cn("inline-flex items-center rounded-full border px-2.5 py-0.5 text-xs font-medium", tones[tone], className)}
      {...props}
    />
  );
}
