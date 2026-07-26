import { useMemo } from "react";
import { useAtomValue } from "jotai";
import { activeCountriesAtom } from "src/atoms/country";
import { localeAtom } from "src/atoms/generic";
import { getCountryName } from "src/utils/country-codes";

// Select options for the country picklist: the activated subset, plus the
// record's current code if it isn't (or is no longer) active — otherwise
// editing a record whose country was later deactivated would silently blank
// it on save.
export function useCountryOptions(currentCode?: string | null) {
  const activeCountries = useAtomValue(activeCountriesAtom);
  const locale = useAtomValue(localeAtom);

  return useMemo(() => {
    const codes = new Set(activeCountries);
    if (currentCode) codes.add(currentCode);
    return Array.from(codes)
      .map((code) => ({ value: code, label: getCountryName(code, locale) }))
      .sort((a, b) => a.label.localeCompare(b.label));
  }, [activeCountries, currentCode, locale]);
}
