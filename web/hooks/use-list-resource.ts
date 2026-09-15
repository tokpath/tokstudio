"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { applyListResult, initialListSnapshot, type ListLoadResult, type ListSnapshot } from "@/lib/list-resource";

export function useListResource<T>({
  load,
  queryKey = "",
  enabled = true,
}: {
  load: () => Promise<ListLoadResult<T>>;
  queryKey?: string;
  enabled?: boolean;
}) {
  const [snapshot, setSnapshot] = useState<ListSnapshot<T>>(initialListSnapshot);
  const [refreshing, setRefreshing] = useState(false);
  const loadRef = useRef(load);
  loadRef.current = load;
  const snapshotRef = useRef(snapshot);
  snapshotRef.current = snapshot;

  const reload = useCallback(async () => {
    if (snapshotRef.current.items.length > 0) {
      setRefreshing(true);
    }
    try {
      const result = await loadRef.current();
      setSnapshot((prev) => applyListResult(prev, result));
    } catch {
      setSnapshot((prev) => applyListResult(prev, { ok: false, network: true }));
    } finally {
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    if (!enabled) {
      return;
    }
    setSnapshot(initialListSnapshot());
    void reload();
  }, [enabled, queryKey, reload]);

  return { snapshot, refreshing, reload };
}
