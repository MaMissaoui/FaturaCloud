import { describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { MemoryRouter, useNavigate } from "react-router";

import { useLoadOnPath } from "./useLoadOnPath";

// A load the test resolves by hand, one per call.
const deferredLoads = () => {
  const pending: Array<() => void> = [];
  const load = vi.fn(
    () =>
      new Promise<void>((resolve) => {
        pending.push(resolve);
      }),
  );
  return { load, pending };
};

const renderOnPath = (initialPath: string, load: () => Promise<unknown>) => {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <MemoryRouter initialEntries={[initialPath]}>{children}</MemoryRouter>
  );
  return renderHook(
    // An inline load, as the pages pass one: a new function every render.
    ({ tick }: { tick: number }) => {
      const navigate = useNavigate();
      const loading = useLoadOnPath("/clients", () => load());
      return { loading, navigate, tick };
    },
    { wrapper, initialProps: { tick: 0 } },
  );
};

const flush = () => act(async () => {});

describe("useLoadOnPath", () => {
  it("loads once per location, even when load is a new function every render", async () => {
    const { load, pending } = deferredLoads();
    const { result, rerender } = renderOnPath("/clients", load);

    expect(result.current.loading).toBe(true);
    rerender({ tick: 1 });
    rerender({ tick: 2 });
    expect(load).toHaveBeenCalledTimes(1);

    await act(async () => pending[0]());
    expect(result.current.loading).toBe(false);

    // Opening a drawer changes the location (router state) — that refetches.
    act(() => result.current.navigate("/clients", { state: { clientModal: true } }));
    expect(load).toHaveBeenCalledTimes(2);
    expect(result.current.loading).toBe(true);
    await act(async () => pending[1]());
    expect(result.current.loading).toBe(false);
  });

  it("doesn't let a superseded load finish the newer location's", async () => {
    const { load, pending } = deferredLoads();
    const { result } = renderOnPath("/clients", load);

    act(() => result.current.navigate("/clients", { state: { clientModal: true } }));
    expect(load).toHaveBeenCalledTimes(2);

    await act(async () => pending[0]()); // the first location's load returns late
    expect(result.current.loading).toBe(true);
    await act(async () => pending[1]());
    expect(result.current.loading).toBe(false);
  });

  it("neither loads nor reports loading off its path", async () => {
    const { load } = deferredLoads();
    const { result } = renderOnPath("/clients/abc", load);
    await flush();

    expect(load).not.toHaveBeenCalled();
    expect(result.current.loading).toBe(false);
  });
});

describe("useLoadOnPath deps", () => {
  it("reloads, and reports loading, when a dep changes", async () => {
    const { load, pending } = deferredLoads();
    const wrapper = ({ children }: { children: ReactNode }) => (
      <MemoryRouter initialEntries={["/products"]}>{children}</MemoryRouter>
    );
    const { result, rerender } = renderHook(
      ({ page }: { page: number }) => useLoadOnPath("/products", () => load(), [page, "sku"]),
      { wrapper, initialProps: { page: 1 } },
    );
    await act(async () => pending[0]());
    expect(result.current).toBe(false);

    rerender({ page: 1 }); // same values: no reload
    expect(load).toHaveBeenCalledTimes(1);

    rerender({ page: 2 });
    expect(load).toHaveBeenCalledTimes(2);
    expect(result.current).toBe(true);
    await act(async () => pending[1]());
    expect(result.current).toBe(false);
  });
});
