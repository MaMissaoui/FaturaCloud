// Proposes a product code (SKU) for a new product, in the PREFIX-NNN shape
// (e.g. "MAL-016") a catalog is usually numbered in.
//
// 1. Learn from the catalog: find the existing product whose name shares
//    the longest run of leading words with the new name and whose code is
//    already PREFIX-NNN — a new "Machine à laver Condor 8kg" next to
//    "Machine à laver HGE 9kg" (MAL-003) continues the MAL- series.
// 2. Otherwise start a series from the name's first word: its first three
//    letters, accents removed ("Réfrigérateur …" -> "REF").
//
// Either way the number is one past the highest already used with that
// prefix, zero-padded to that series' width (3 digits for a new series), so
// the proposal never collides with an existing code.

export interface ProductCodeSource {
  name?: string | null;
  sku?: string | null;
}

const SERIES_CODE = /^([A-Z0-9]+)-(\d+)$/;

// Words compared case- and accent-insensitively; punctuation separates.
export const normalizeWords = (text: string): string[] =>
  text
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .toUpperCase()
    .split(/[^A-Z0-9]+/)
    .filter(Boolean);

const leadingWordsInCommon = (a: string[], b: string[]): number => {
  let n = 0;
  while (n < a.length && n < b.length && a[n] === b[n]) n += 1;
  return n;
};

const nextInSeries = (prefix: string, codes: string[]): string => {
  let highest = 0;
  let width = 3;
  for (const code of codes) {
    const m = SERIES_CODE.exec(code);
    if (m && m[1] === prefix) {
      highest = Math.max(highest, Number(m[2]));
      width = Math.max(width, m[2].length);
    }
  }
  return `${prefix}-${String(highest + 1).padStart(width, "0")}`;
};

export const proposeProductCode = (name: string, products: ProductCodeSource[]): string => {
  const words = normalizeWords(name);
  const codes = products.map((p) => (p.sku ?? "").trim().toUpperCase()).filter(Boolean);

  // A single shared word only counts when it has 2+ characters — "TV 32"
  // and "TV 55" are a series, but a stray "A" is not.
  let bestPrefix = "";
  let bestShared = 0;
  for (const p of products) {
    const m = SERIES_CODE.exec((p.sku ?? "").trim().toUpperCase());
    if (!m || !p.name) continue;
    const shared = leadingWordsInCommon(words, normalizeWords(p.name));
    const meaningful = shared > 1 || (shared === 1 && words[0].length >= 2);
    if (meaningful && shared > bestShared) {
      bestShared = shared;
      bestPrefix = m[1];
    }
  }

  const prefix = bestPrefix || (words[0] ?? "").slice(0, 3) || "PRD";
  return nextInSeries(prefix, codes);
};
