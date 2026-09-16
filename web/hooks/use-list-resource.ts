"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { applyListResult, initialListSnapshot, type ListLoadResult, type ListSnapshot } from "@/lib/list-resource";

export function useListResource<T>({
  load,
  queryKey = "",
  enabled = true,
  onAccepted,
}: {
  load: () => Promise<ListLoadResult<T>>;
  queryKey?: string;
  enabled?: boolean;
  onAccepted?: (result: ListLoadResult<T>) => void;
}) {
  const [snapshot, setSnapshot] = useState<ListSnapshot<T>>(initialListSnapshot);
  const [refreshing, setRefreshing] = useState(false);
  const loadRef = useRef(load);
  loadRef.current = load;
  const snapshotRef = useRef(snapshot);
  snapshotRef.current = snapshot;
  const queryKeyRef = useRef(queryKey);
  queryKeyRef.current = queryKey;
  const onAcceptedRef = useRef(onAccepted);
  onAcceptedRef.current = onAccepted;
  const seqRef = useRef(0);

  const reload = useCallback(async () => {
    const seq = ++seqRef.current;
    const key = queryKeyRef.current;
    if (snapshotRef.current.items.length > 0) {
      setRefreshing(true);
    }
    try {
      const result = await loadRef.current();
      if (seq !== seqRef.current || key !== queryKeyRef.current) {
        return;
      }
      setSnapshot((prev) => applyListResult(prev, result));
      onAcceptedRef.current?.(result);
    } catch {
      if (seq !== seqRef.current || key !== queryKeyRef.current) {
        return;
      }
      setSnapshot((prev) => applyListResult(prev, { ok: false, network: true }));
    } finally {
      if (seq === seqRef.current) {
        setRefreshing(false);
      }
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
