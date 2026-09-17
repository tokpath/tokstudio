"use client";

import { useTranslations } from "next-intl";
import type { FieldValues, UseFormHandleSubmit } from "react-hook-form";
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
import { useConfirmSession, type ConfirmResult } from "@/components/use-confirm-session";

export type { ConfirmResult };

export type ConfirmButtonProps = Omit<ButtonProps, "onClick" | "type"> & {
  title: string;
  description?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  validate?: () => boolean | Promise<boolean>;
  /** Only an explicit `true` closes the dialog. `void`, `false`, and thrown errors stay open. */
  onConfirm: () => ConfirmResult;
};

/** RHF `handleSubmit` swallows the inner return value; wrap it so callers can signal success. */
export function confirmFormSubmit<T extends FieldValues>(
  handleSubmit: UseFormHandleSubmit<T>,
  onValid: (values: T) => Promise<boolean>,
): () => Promise<boolean> {
  return async () => {
    let outcome = false;
    await handleSubmit(async (values) => {
      outcome = (await onValid(values)) === true;
    })();
    return outcome;
  };
}

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
  const { open, pending, handleOpenChange, dismiss, runConfirm } = useConfirmSession(onConfirm);

  async function openDialog() {
    if (validate) {
      const ok = await validate();
      if (!ok) {
        return;
      }
    }
    handleOpenChange(true);
  }

  return (
    <>
      <Button type="button" disabled={disabled || pending} onClick={openDialog} {...buttonProps}>
        {children}
      </Button>
      <Dialog open={open} onOpenChange={handleOpenChange}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle className="inline-flex items-center gap-2">
              <AlertTriangle className="size-4 text-hold" strokeWidth={1.75} aria-hidden />
              {title}
            </DialogTitle>
            {description ? <DialogDescription>{description}</DialogDescription> : null}
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => dismiss()}>
              {cancelText}
            </Button>
            <Button type="button" disabled={pending} onClick={() => void runConfirm()}>
              {confirmText}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
