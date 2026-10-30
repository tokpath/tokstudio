"use client";
import { useQuery } from "@tanstack/react-query";
import { request } from "@/lib/api-client";
import { UnreadCountSchema } from "@/lib/contracts";

// Counts notices addressed to the signed-in person. A failed read leaves the
// bell unmarked rather than showing a stale or guessed number.
export function useUnreadNotifications(path?: string) {
  const query = useQuery({
    queryKey: ["notification-unread", path],
    queryFn: () => request(path!, UnreadCountSchema),
    enabled: Boolean(path),
    refetchInterval: 5_000,
  });
  return query.data?.count ?? 0;
}

export function UnreadBadge({ count }: { count: number }) {
  if (count < 1) return null;
  const shown = count > 99 ? "99+" : String(count);
  return <span aria-hidden="true" className="absolute -right-2 -top-2 grid min-h-4 min-w-4 place-items-center rounded-full bg-danger px-1 text-[10px] font-semibold leading-4 text-[var(--on-danger)]">{shown}</span>;
}
