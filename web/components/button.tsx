import type { ReactNode } from "react";

type Variant = "primary" | "secondary" | "ghost";

const variants: Record<Variant, string> = {
  primary: "bg-brand text-on-brand hover:bg-brand-press",
  secondary: "border border-hairline bg-transparent text-ink hover:bg-canvas-raised",
  ghost: "bg-transparent text-ink-secondary hover:text-ink",
};

const baseClass =
  "inline-flex min-h-10 items-center justify-center rounded-stamp px-5 text-sm font-medium no-underline max-sm:min-h-11";

export function Button({
  href,
  variant = "secondary",
  children,
  className = "",
  type = "button",
  onClick,
}: {
  href?: string;
  variant?: Variant;
  children: ReactNode;
  className?: string;
  type?: "button" | "submit";
  onClick?: () => void;
}) {
  const classNameAll = `${baseClass} ${variants[variant]} ${className}`;
  if (href) {
    return (
      <a href={href} className={classNameAll}>
        {children}
      </a>
    );
  }
  return (
    <button type={type} onClick={onClick} className={classNameAll}>
      {children}
    </button>
  );
}
