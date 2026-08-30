import type { ReactNode } from "react";

type Variant = "primary" | "secondary" | "ghost";

const variants: Record<Variant, string> = {
  primary: "bg-brand text-on-brand hover:bg-brand-press",
  secondary: "border border-hairline bg-transparent text-ink hover:bg-canvas-raised",
  ghost: "bg-transparent text-ink-secondary hover:text-ink",
};

export function Button({
  href,
  variant = "secondary",
  children,
  className = "",
}: {
  href: string;
  variant?: Variant;
  children: ReactNode;
  className?: string;
}) {
  return (
    <a
      href={href}
      className={`inline-flex min-h-10 items-center justify-center rounded-stamp px-4 text-sm font-medium no-underline max-sm:min-h-11 ${variants[variant]} ${className}`}
    >
      {children}
    </a>
  );
}
