import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-stamp text-sm font-medium leading-tight transition-colors duration-150 ease-out disabled:pointer-events-none disabled:opacity-40 max-sm:min-h-11 [&_svg]:pointer-events-none [&_svg]:size-3.5 [&_svg]:shrink-0",
  {
    variants: {
      variant: {
        default: "bg-brand text-on-brand hover:bg-brand-press",
        outline: "border border-hairline bg-transparent text-ink hover:bg-canvas-raised",
        ghost: "bg-transparent text-ink-secondary hover:bg-canvas-raised hover:text-ink",
        destructive: "bg-danger text-on-brand hover:opacity-90",
        secondary: "border border-hairline bg-transparent text-ink hover:bg-canvas-raised",
        link: "h-auto min-h-0 px-0 py-0 text-brand-emphasis underline-offset-4 hover:underline",
      },
      size: {
        default: "min-h-10 px-5 py-2.5",
        sm: "min-h-10 px-4 py-2",
        lg: "min-h-11 px-8",
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
    return <Comp className={cn(buttonVariants({ variant, size, className }))} ref={ref} {...props} />;
  },
);
Button.displayName = "Button";

export { buttonVariants };
