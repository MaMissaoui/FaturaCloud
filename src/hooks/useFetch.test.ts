import { describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";

import { useFetch } from "./useFetch";

// A fetcher the test settles by hand, one promise per call.
const deferredFetches = <T>() => {
  const calls: Array<{ resolve: (v: T) => void; reject: (e: unknown) => void }> = [];
  const fetcher = vi.fn(
    () =>
      new Promise<T>((resolve, reject) => {
        calls.push({ resolve, reject });
      }),
  );
  return { fetcher, calls };
};

describe("useFetch", () => {
  it("fetches once per key, keeps the last data while reloading, and derives loading", async () => {
    const { fetcher, calls } = deferredFetches<string[]>();
    const { result, rerender } = renderHook(
      ({ range }: { range: number }) => useFetch(["org-1", range], () => fetcher(), []),
      { initialProps: { range: 1 } },
    );
    expect(result.current).toMatchObject({ loading: true, failed: false, data: [] });

    rerender({ range: 1 }); // same key, new inline fetcher: no refetch
    expect(fetcher).toHaveBeenCalledTimes(1);
    await act(async () => calls[0].resolve(["a"]));
    expect(result.current).toMatchObject({ loading: false, failed: false, data: ["a"] });

    rerender({ range: 2 });
    expect(fetcher).toHaveBeenCalledTimes(2);
    expect(result.current).toMatchObject({ loading: true, data: ["a"] });
    await act(async () => calls[1].resolve(["b"]));
    expect(result.current).toMatchObject({ loading: false, data: ["b"] });
  });

  it("drops a superseded key's response", async () => {
    const { fetcher, calls } = deferredFetches<string>();
    const { result, rerender } = renderHook(
      ({ range }: { range: number }) => useFetch([range], () => fetcher(), ""),
      { initialProps: { range: 1 } },
    );
    rerender({ range: 2 });
    await act(async () => calls[0].resolve("old")); // range 1 answers late
    expect(result.current).toMatchObject({ loading: true, data: "" });
    await act(async () => calls[1].resolve("new"));
    expect(result.current).toMatchObject({ loading: false, data: "new" });
  });

  it("reports a failure with empty data, and reload retries the same key", async () => {
    const { fetcher, calls } = deferredFetches<number>();
    const { result } = renderHook(() => useFetch(["org-1"], () => fetcher(), 0));
    await act(async () => calls[0].reject(new Error("500")));
    expect(result.current).toMatchObject({ loading: false, failed: true, data: 0 });

    act(() => result.current.reload());
    expect(fetcher).toHaveBeenCalledTimes(2);
    expect(result.current).toMatchObject({ loading: true, failed: false });
    await act(async () => calls[1].resolve(7));
    expect(result.current).toMatchObject({ loading: false, failed: false, data: 7 });
  });

  it("does nothing while the key is null", async () => {
    const { fetcher } = deferredFetches<number>();
    const { result } = renderHook(() => useFetch(null, () => fetcher(), 0));
    await act(async () => {});
    expect(fetcher).not.toHaveBeenCalled();
    expect(result.current).toMatchObject({ loading: false, failed: false, data: 0 });
  });
});
