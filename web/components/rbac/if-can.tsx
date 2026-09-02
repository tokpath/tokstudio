"use client";

import type { ReactNode } from "react";
import { canWrite, shouldBypassRbac, type WriteAction } from "@/lib/rbac";
import { useViewer } from "./viewer-context";

export function IfCan({ action, children }: { action: WriteAction; children: ReactNode }) {
  const viewer = useViewer();
  if (!canWrite(action, viewer)) {
    return null;
  }
  return <>{children}</>;
}

export function IfRoles({ roles, children }: { roles: readonly string[]; children: ReactNode }) {
  const viewer = useViewer();
  if (shouldBypassRbac(viewer) || viewer.roles.some((role) => roles.includes(role))) {
    return <>{children}</>;
  }
  return null;
}
