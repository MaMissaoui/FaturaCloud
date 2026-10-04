import { useSyncExternalStore } from "react";

const WIDE = "(min-width: 1440px)";

const subscribe = (onChange: () => void) => {
  const query = window.matchMedia(WIDE);
  query.addEventListener("change", onChange);
  return () => query.removeEventListener("change", onChange);
};

// useWideScreen reports whether the window is at least 1440px wide, where a
// master-data list has room for its secondary columns (City, Code, Sale
// price) beside the summary panel. Below that the column is not rendered at
// all: hiding its cells with CSS left antd's measure row holding the column's
// width, so the table scrolled sideways at 1280px with an empty strip.
export function useWideScreen(): boolean {
  return useSyncExternalStore(
    subscribe,
    () => window.matchMedia(WIDE).matches,
    () => true,
  );
}
