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
import { SubmitStatus } from "@/components/console/submit-status";
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

function ConfirmDialogView({
  open,
  onOpenChange,
  pending,
  dismiss,
  runConfirm,
  title,
  description,
  error,
  confirmText,
  cancelText,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pending: boolean;
  dismiss: () => void;
  runConfirm: () => void;
  title: string;
  description?: string;
  error?: string;
  confirmText: string;
  cancelText: string;
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className="inline-flex items-center gap-2">
            <AlertTriangle className="size-4 text-hold" strokeWidth={1.75} aria-hidden />
            {title}
          </DialogTitle>
          {description ? <DialogDescription>{description}</DialogDescription> : null}
        </DialogHeader>
        {error ? <SubmitStatus error={error} /> : null}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => dismiss()}>
            {cancelText}
          </Button>
          <Button type="button" disabled={pending} onClick={() => runConfirm()}>
            {confirmText}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function ConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  error,
  confirmLabel,
  cancelLabel,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  error?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  onConfirm: () => ConfirmResult;
}) {
  const t = useTranslations("common");
  const { pending, handleOpenChange, dismiss, runConfirm } = useConfirmSession(onConfirm, open, onOpenChange);
  return (
    <ConfirmDialogView
      open={open}
      onOpenChange={handleOpenChange}
      pending={pending}
      dismiss={dismiss}
      runConfirm={() => void runConfirm()}
      title={title}
      description={description}
      error={error}
      confirmText={confirmLabel ?? t("confirm")}
      cancelText={cancelLabel ?? t("cancel")}
    />
  );
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
      <ConfirmDialogView
        open={open}
        onOpenChange={handleOpenChange}
        pending={pending}
        dismiss={dismiss}
        runConfirm={() => void runConfirm()}
        title={title}
        description={description}
        confirmText={confirmText}
        cancelText={cancelText}
      />
    </>
  );
}
