import { atom } from "jotai";
import type { Atom } from "jotai";
import { unwrap } from "jotai/utils";

export type Loadable<Value> =
  | { state: "loading" }
  | { state: "hasError"; error: unknown }
  | { state: "hasData"; data: Awaited<Value> };

const LOADING = { state: "loading" } as const;

// jotai's own `loadable` is deprecated in favor of a userland implementation
// built on `unwrap` (still non-deprecated) — this is that implementation,
// copied from jotai's deprecation notice so every call site keeps the same
// synchronous-resolve behavior without the removal warning.
export function loadable<Value>(anAtom: Atom<Value>): Atom<Loadable<Value>> {
  const unwrappedAtom = unwrap(anAtom, () => LOADING);
  return atom((get) => {
    try {
      const data = get(unwrappedAtom);
      if (data === LOADING) {
        return LOADING;
      }
      return { state: "hasData", data } as Loadable<Value>;
    } catch (error) {
      return { state: "hasError", error };
    }
  });
}
