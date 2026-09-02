"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { emptyViewer, type Viewer } from "@/lib/rbac";

const ViewerContext = createContext<Viewer | null>(null);

type MeBody = { user?: { id?: string; roles?: string[] } };

async function loadViewer(): Promise<Viewer> {
  try {
    const [meRes, partnerRes] = await Promise.all([
      fetch("/v1/me", { credentials: "include" }),
      fetch("/v1/partner/me", { credentials: "include" }),
    ]);
    if (!meRes.ok) {
      return { signedIn: false, loading: false, roles: [] };
    }
    const body = (await meRes.json()) as MeBody;
    return {
      signedIn: true,
      loading: false,
      roles: body.user?.roles ?? [],
      userId: body.user?.id,
      isPartner: partnerRes.ok,
    };
  } catch {
    return { signedIn: false, loading: false, roles: [] };
  }
}

export function ViewerProvider({ children }: { children: ReactNode }) {
  const [viewer, setViewer] = useState<Viewer>(emptyViewer);

  useEffect(() => {
    let cancelled = false;
    void loadViewer().then((next) => {
      if (!cancelled) {
        setViewer(next);
      }
    });
    return () => {
      cancelled = true;
    };
  }, []);

  return <ViewerContext.Provider value={viewer}>{children}</ViewerContext.Provider>;
}

export function useViewer(): Viewer {
  return useContext(ViewerContext) ?? emptyViewer;
}
