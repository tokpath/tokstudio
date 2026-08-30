"use client";

import { useState } from "react";
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
  confirmLabel = "确认",
  cancelLabel = "取消",
  validate,
  onConfirm,
  children,
  disabled,
  ...buttonProps
}: ConfirmButtonProps) {
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
            <DialogTitle>{title}</DialogTitle>
            {description ? <DialogDescription>{description}</DialogDescription> : null}
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" disabled={pending} onClick={() => setOpen(false)}>
              {cancelLabel}
            </Button>
            <Button type="button" disabled={pending} onClick={handleConfirm}>
              {confirmLabel}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
