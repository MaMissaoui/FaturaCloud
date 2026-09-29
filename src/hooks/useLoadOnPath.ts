import { useEffect, useEffectEvent, useState } from "react";
import { useLocation, type Location } from "react-router";

type Primitive = string | number | boolean | null | undefined;

// useLoadOnPath runs load() on every location change while the pathname is
// `path` — including a drawer opening or closing through router state, which
// is how the list pages refetch after a create/update/delete — and returns
// whether the current location's load is still running. `deps` are further
// values that re-run the load when they change (a paginated list's page, sort
// and filters); they are part of what "the current load" means, so changing
// one shows loading too. Primitives only, so they can be compared by value.
//
// Loading is derived (the load for this exact location and deps hasn't
// finished yet) rather than set to true inside the effect: React's
// set-state-in-effect rule flags that, since it forces an extra render. A
// superseded load never marks the newer one as done. `load` may be an inline
// function — it's read through useEffectEvent, so a new identity on every
// render doesn't refetch.
export function useLoadOnPath(
  path: string,
  load: () => Promise<unknown> | void,
  deps: readonly Primitive[] = [],
): boolean {
  const location = useLocation();
  const onPath = location.pathname === path;
  const depsKey = JSON.stringify(deps);
  const [loadedFor, setLoadedFor] = useState<{ location: Location; depsKey: string } | null>(null);
  const runLoad = useEffectEvent(() => load());

  useEffect(() => {
    if (!onPath) return;
    let active = true;
    Promise.resolve(runLoad()).finally(() => {
      if (active) setLoadedFor({ location, depsKey });
    });
    return () => {
      active = false;
    };
  }, [onPath, location, depsKey]);

  return onPath && !(loadedFor?.location === location && loadedFor.depsKey === depsKey);
}
