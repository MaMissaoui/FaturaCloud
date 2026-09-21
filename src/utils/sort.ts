// Shared comparators for antd table column `sorter`s. Antd calls the sorter
// with two rows and expects a number; these wrap the common field types so
// every list page sorts consistently (case-insensitive text with numeric
// segments, numeric money/quantity, epoch-millisecond dates, and status by
// its label).

export function textSorter<T>(get: (row: T) => string | null | undefined) {
  return (a: T, b: T) =>
    String(get(a) ?? "").localeCompare(String(get(b) ?? ""), undefined, {
      numeric: true,
      sensitivity: "base",
    });
}

export function numberSorter<T>(get: (row: T) => number | null | undefined) {
  return (a: T, b: T) => (get(a) ?? 0) - (get(b) ?? 0);
}

export function dateSorter<T>(get: (row: T) => number | string | null | undefined) {
  const toMs = (v: number | string | null | undefined) => {
    if (v == null) return 0;
    if (typeof v === "number") return v;
    const t = Date.parse(v);
    return Number.isNaN(t) ? 0 : t;
  };
  return (a: T, b: T) => toMs(get(a)) - toMs(get(b));
}

// Money is stored in integer cents everywhere, so a plain numeric compare is
// correct — exposed as its own name for call-site readability.
export function moneySorter<T>(get: (row: T) => number | null | undefined) {
  return numberSorter(get);
}
