"use client";

import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { apiBase } from "@/lib/api";
import { emptyViewer, type Viewer } from "@/lib/rbac";

const ViewerContext = createContext<Viewer | null>(null);

type MeBody = { user?: { id?: string; roles?: string[] } };

async function loadViewer(): Promise<Viewer> {
  try {
    const [meRes, partnerRes] = await Promise.allSettled([
      fetch(`${apiBase}/v1/me`, { credentials: "include" }),
      fetch(`${apiBase}/v1/partner/me`, { credentials: "include" }),
    ]);
    if (meRes.status === "rejected") {
      return { signedIn: false, loading: false, roles: [], error: true };
    }
    if (!meRes.value.ok) {
      return { signedIn: false, loading: false, roles: [], error: meRes.value.status !== 401 };
    }
    const body = (await meRes.value.json()) as MeBody;
    return {
      signedIn: true,
      loading: false,
      roles: body.user?.roles ?? [],
      userId: body.user?.id,
      isPartner: partnerRes.status === "fulfilled" && partnerRes.value.ok,
      partnerError: partnerRes.status === "rejected" || (partnerRes.status === "fulfilled" && !partnerRes.value.ok && ![401, 403].includes(partnerRes.value.status)),
    };
  } catch {
    return { signedIn: false, loading: false, roles: [], error: true };
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
