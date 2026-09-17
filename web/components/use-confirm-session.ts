"use client";

import { useCallback, useRef, useState } from "react";

export type ConfirmResult = boolean | void | Promise<boolean | void>;

export function isConfirmSuccess(value: unknown): boolean {
  return value === true;
}

export function useConfirmSession(onConfirm: () => unknown | Promise<unknown>) {
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const pendingRef = useRef(false);
  const generationRef = useRef(0);

  const dismiss = useCallback(() => {
    if (pendingRef.current) {
      generationRef.current += 1;
    }
    pendingRef.current = false;
    setPending(false);
    setOpen(false);
  }, []);

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) {
        dismiss();
        return;
      }
      setOpen(true);
    },
    [dismiss],
  );

  const runConfirm = useCallback(async () => {
    if (pendingRef.current) {
      return;
    }
    const generation = generationRef.current;
    pendingRef.current = true;
    setPending(true);
    try {
      const result = await onConfirm();
      if (generation !== generationRef.current) {
        return;
      }
      if (isConfirmSuccess(result)) {
        pendingRef.current = false;
        setPending(false);
        setOpen(false);
      }
    } catch {
      // Keep the dialog open; callers report network/API errors themselves.
    } finally {
      if (generation === generationRef.current) {
        pendingRef.current = false;
        setPending(false);
      }
    }
  }, [onConfirm]);

  return { open, pending, handleOpenChange, dismiss, runConfirm };
}
