"use client";

import { useCallback, useEffect, useRef, useState } from "react";

export type ConfirmResult = boolean | void | Promise<boolean | void>;

export function isConfirmSuccess(value: unknown): boolean {
  return value === true;
}

export function useConfirmSession(
  onConfirm: () => unknown | Promise<unknown>,
  controlledOpen?: boolean,
  onControlledOpenChange?: (open: boolean) => void,
) {
  const [internalOpen, setInternalOpen] = useState(false);
  const [pending, setPending] = useState(false);
  const pendingRef = useRef(false);
  const generationRef = useRef(0);
  const controlled = controlledOpen !== undefined;
  const open = controlled ? controlledOpen : internalOpen;
  const onControlledOpenChangeRef = useRef(onControlledOpenChange);
  onControlledOpenChangeRef.current = onControlledOpenChange;

  const setOpen = useCallback(
    (next: boolean) => {
      if (controlled) {
        onControlledOpenChangeRef.current?.(next);
        return;
      }
      setInternalOpen(next);
    },
    [controlled],
  );

  const dismiss = useCallback(() => {
    if (pendingRef.current) {
      generationRef.current += 1;
    }
    pendingRef.current = false;
    setPending(false);
    setOpen(false);
  }, [setOpen]);

  useEffect(() => {
    if (!controlled || open) {
      return;
    }
    if (pendingRef.current) {
      generationRef.current += 1;
    }
    pendingRef.current = false;
    setPending(false);
  }, [controlled, open]);

  const handleOpenChange = useCallback(
    (next: boolean) => {
      if (!next) {
        dismiss();
        return;
      }
      setOpen(true);
    },
    [dismiss, setOpen],
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
  }, [onConfirm, setOpen]);

  return { open, pending, handleOpenChange, dismiss, runConfirm };
}
