/** @vitest-environment jsdom */
import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useListResource } from "./use-list-resource";
import type { ListLoadResult } from "@/lib/list-resource";

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

describe("useListResource request ownership", () => {
  it("keeps the later query when an earlier request finishes last", async () => {
    const first = deferred<ListLoadResult<{ id: string }>>();
    const second = deferred<ListLoadResult<{ id: string }>>();
    const { result, rerender } = renderHook(
      ({ queryKey, load }) => useListResource({ queryKey, load }),
      { initialProps: { queryKey: "A", load: () => first.promise } },
    );

    rerender({ queryKey: "B", load: () => second.promise });
    await act(async () => {
      second.resolve({ ok: true, items: [{ id: "b" }] });
    });
    await waitFor(() => expect(result.current.snapshot.items).toEqual([{ id: "b" }]));

    await act(async () => {
      first.resolve({ ok: true, items: [{ id: "a" }] });
    });
    expect(result.current.snapshot.items).toEqual([{ id: "b" }]);
    expect(result.current.snapshot.phase).toBe("ready");
  });

  it("ignores a late failure from a superseded request", async () => {
    const first = deferred<ListLoadResult<{ id: string }>>();
    const second = deferred<ListLoadResult<{ id: string }>>();
    const { result, rerender } = renderHook(
      ({ queryKey, load }) => useListResource({ queryKey, load }),
      { initialProps: { queryKey: "A", load: () => first.promise } },
    );

    rerender({ queryKey: "B", load: () => second.promise });
    await act(async () => {
      second.resolve({ ok: true, items: [{ id: "b" }] });
    });
    await waitFor(() => expect(result.current.snapshot.phase).toBe("ready"));

    await act(async () => {
      first.resolve({ ok: false, status: 500, message: "late" });
    });
    expect(result.current.snapshot.phase).toBe("ready");
    expect(result.current.snapshot.items).toEqual([{ id: "b" }]);
    expect(result.current.snapshot.message).toBe("");
  });

  it("does not apply companion extras from a superseded request", async () => {
    const first = deferred<ListLoadResult<{ id: string }>>();
    const second = deferred<ListLoadResult<{ id: string }>>();
    const accepted: unknown[] = [];
    const { rerender } = renderHook(
      ({ queryKey, load }) => useListResource({ queryKey, load, onAccepted: (result) => accepted.push(result.extras) }),
      { initialProps: { queryKey: "A", load: () => first.promise } },
    );

    rerender({ queryKey: "B", load: () => second.promise });
    await act(async () => {
      second.resolve({ ok: true, items: [{ id: "b" }], extras: { label: "B" } });
    });
    await act(async () => {
      first.resolve({ ok: true, items: [{ id: "a" }], extras: { label: "A" } });
    });
    expect(accepted).toEqual([{ label: "B" }]);
  });

  it("clears loaded rows on a later 401", async () => {
    let calls = 0;
    const { result } = renderHook(() =>
      useListResource({
        load: async () => {
          calls += 1;
          if (calls === 1) {
            return { ok: true, items: [{ id: "u1" }] };
          }
          return { ok: false, status: 401, code: "authentication_error", message: "未登录" };
        },
      }),
    );
    await waitFor(() => expect(result.current.snapshot.phase).toBe("ready"));

    await act(async () => {
      await result.current.reload();
    });
    expect(result.current.snapshot.phase).toBe("unauthorized");
    expect(result.current.snapshot.auth).toBe("session");
    expect(result.current.snapshot.items).toEqual([]);
  });
});
