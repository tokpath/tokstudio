"use client";

import { useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { Button, type ButtonProps } from "@/components/ui/button";
import {
  Dialog,
  DialogDescription,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
} from "@/components/ui/dialog";
import * as DialogPrimitive from "@radix-ui/react-dialog";
import { cn } from "@/lib/utils";

export type SealConfirmProps = Omit<ButtonProps, "onClick" | "type"> & {
  title: string;
  description?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  validate?: () => boolean | Promise<boolean>;
  onConfirm: () => void | Promise<void>;
};

/** DESIGN.md 1.0 高风险盖章：蒙层 + 抬起卡片 + 后果标题 + 实心确认章。点蒙层/Esc 只取消。 */
export function SealConfirm({
  title,
  description,
  confirmLabel,
  cancelLabel,
  validate,
  onConfirm,
  children,
  disabled,
  ...buttonProps
}: SealConfirmProps) {
  const t = useTranslations("common");
  const confirmText = confirmLabel ?? "盖章确认";
  const cancelText = cancelLabel ?? t("cancel");
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const titleRef = useRef<HTMLHeadingElement>(null);

  async function openSeal() {
    if (validate) {
      const ok = await validate();
      if (!ok) {
        return;
      }
    }
    setOpen(true);
  }

  async function handleConfirm() {
    setPending(true);
    try {
      await onConfirm();
      setOpen(false);
    } finally {
      setPending(false);
    }
  }

  return (
    <>
      <Button type="button" disabled={disabled || pending} onClick={openSeal} {...buttonProps}>
        {children}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogPortal>
          <DialogOverlay />
          <DialogPrimitive.Content
            className={cn(
              "fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 rounded-card border border-hairline bg-canvas-raised p-8 text-ink",
            )}
            onOpenAutoFocus={(event) => {
              event.preventDefault();
              titleRef.current?.focus();
            }}
          >
            <p className="font-mono text-[11px] font-medium uppercase tracking-[0.08em] text-danger">SEAL</p>
            <DialogTitle
              ref={titleRef}
              tabIndex={-1}
              className="mt-3 text-xl font-semibold leading-snug tracking-tight outline-none"
            >
              {title}
            </DialogTitle>
            {description ? <DialogDescription className="mt-3">{description}</DialogDescription> : null}
            <div className="mt-8 flex flex-row flex-wrap justify-end gap-2">
              <Button type="button" variant="outline" disabled={pending} onClick={() => setOpen(false)}>
                {cancelText}
              </Button>
              <Button type="button" variant="destructive" disabled={pending} onClick={handleConfirm}>
                {confirmText}
              </Button>
            </div>
          </DialogPrimitive.Content>
        </DialogPortal>
      </Dialog>
    </>
  );
}
