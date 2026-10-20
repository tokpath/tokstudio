import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-2 whitespace-normal text-center rounded-stamp text-sm font-medium leading-tight transition-[color,background-color,border-color,box-shadow] duration-150 ease-out focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-brand disabled:pointer-events-none disabled:opacity-40 max-sm:min-h-11 [&_svg]:pointer-events-none [&_svg]:size-3.5 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        default: "border border-transparent bg-brand text-on-brand shadow-[0_1px_2px_rgb(15_28_48_/_0.08)] hover:bg-brand-press hover:text-[var(--on-brand-press)]",
        outline: "border border-hairline bg-canvas-raised text-ink hover:border-brand/30 hover:bg-canvas",
        ghost: "bg-transparent text-ink-secondary hover:bg-canvas-raised hover:text-ink",
        destructive: "bg-danger text-[var(--on-danger)] hover:opacity-90",
        secondary: "border border-transparent bg-brand-soft text-brand-emphasis hover:border-brand/20",
        link: "h-auto min-h-0 px-0 py-0 text-brand-emphasis underline-offset-4 hover:underline",
      },
      size: {
        default: "min-h-11 px-4 py-2.5",
        sm: "min-h-9 px-3 py-2",
        lg: "min-h-12 px-6 py-3",
        icon: "size-10",
      },
    },
    defaultVariants: { variant: "default", size: "default" },
  },
);

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement>, VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : "button";
    return <Comp className={cn(buttonVariants({ variant, size }), className)} ref={ref} {...props} />;
  },
);
Button.displayName = "Button";

export { buttonVariants };
