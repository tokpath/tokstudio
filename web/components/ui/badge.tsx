import { cn } from "@/lib/utils";

export function Badge({
  className,
  tone = "neutral",
  ...props
}: React.HTMLAttributes<HTMLSpanElement> & { tone?: "neutral" | "brand" | "success" | "warn" }) {
  const tones = {
    neutral: "text-ink-mute",
    brand: "text-brand-emphasis",
    success: "text-success",
    warn: "text-hold",
  };
  return (
    <span className={cn("th-eyebrow inline-flex items-center", tones[tone], className)} {...props} />
  );
}
