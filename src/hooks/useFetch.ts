import { useCallback, useEffect, useEffectEvent, useState } from "react";

type Primitive = string | number | boolean | null | undefined;

export interface FetchState<T> {
  // The latest data — kept while a newer key loads, `empty` after a failure.
  data: T;
  // The current key's request hasn't settled yet.
  loading: boolean;
  // The current key's request failed.
  failed: boolean;
  // Refetch the current key (a Refresh / Retry button).
  reload: () => void;
}

// useFetch loads `fetcher()` whenever `key` changes — the report pages' "one
// request per organization/range/filter" shape — and derives loading and
// failed from which key the last settled response belongs to, instead of
// setting them synchronously inside an effect (React's set-state-in-effect
// rule). A response for a superseded key is dropped, which replaces the
// pages' requestIdRef guards. `key` is a list of primitives (build it from
// ids and `.valueOf()` timestamps, never from objects); null means "nothing
// to load" (e.g. no organization, or a drawer creating a new record) — not
// loading, `empty` data. `fetcher`
// may be an inline function: it's read through useEffectEvent.
export function useFetch<T>(
  key: readonly Primitive[] | null,
  fetcher: () => Promise<T>,
  empty: T,
): FetchState<T> {
  const [nonce, setNonce] = useState(0);
  const keyString = key === null ? null : JSON.stringify([...key, nonce]);
  // What the last settled request returned; `empty` is applied when reading,
  // so it isn't an input the effect would have to depend on.
  const [result, setResult] = useState<
    { key: string; ok: true; data: T } | { key: string; ok: false } | null
  >(null);
  const runFetch = useEffectEvent(() => fetcher());

  useEffect(() => {
    if (keyString === null) return;
    let active = true;
    runFetch()
      .then((data) => {
        if (active) setResult({ key: keyString, ok: true, data });
      })
      .catch(() => {
        if (active) setResult({ key: keyString, ok: false });
      });
    return () => {
      active = false;
    };
  }, [keyString]);

  // A null key has nothing loaded, even if an earlier key had: forget that
  // result during render, so neither "new" (a drawer switching from editing a
  // record) nor the next record opened after it shows the old record's data
  // while its own request is in flight.
  if (keyString === null && result !== null) setResult(null);

  const reload = useCallback(() => setNonce((n) => n + 1), []);
  const settled = keyString !== null && result?.key === keyString;
  return {
    data: keyString !== null && result?.ok ? result.data : empty,
    loading: keyString !== null && !settled,
    failed: settled && !result.ok,
    reload,
  };
}
