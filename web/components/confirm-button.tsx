"use client";

import { useState } from "react";
import { useTranslations } from "next-intl";
import { AlertTriangle } from "lucide-react";
import { Button, type ButtonProps } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

export type ConfirmButtonProps = Omit<ButtonProps, "onClick" | "type"> & {
  title: string;
  description?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  validate?: () => boolean | Promise<boolean>;
  onConfirm: () => void | Promise<void>;
};

export function ConfirmButton({
  title,
  description,
  confirmLabel,
  cancelLabel,
  validate,
  onConfirm,
  children,
  disabled,
  ...buttonProps
}: ConfirmButtonProps) {
  const t = useTranslations("common");
  const confirmText = confirmLabel ?? t("confirm");
  const cancelText = cancelLabel ?? t("cancel");
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState(false);

  async function openDialog() {
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
      <Button type="button" disabled={disabled || pending} onClick={openDialog} {...buttonProps}>
        {children}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="inline-flex items-center gap-2">
              <AlertTriangle className="size-4 text-hold" strokeWidth={1.75} aria-hidden />
              {title}
            </DialogTitle>
            {description ? <DialogDescription>{description}</DialogDescription> : null}
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" disabled={pending} onClick={() => setOpen(false)}>
              {cancelText}
            </Button>
            <Button type="button" disabled={pending} onClick={handleConfirm}>
              {confirmText}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
