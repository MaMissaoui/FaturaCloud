import { atom } from "jotai";
import { message } from "src/utils/message";
import { t } from "@lingui/core/macro";
import { GetActiveCountries, SetCountryActive } from "src/api";

// Global, install-wide list of activated ISO 3166-1 alpha-2 codes — not
// per-organization (the new-organization form has no organization yet).
export const activeCountriesAtom = atom<string[]>([]);

export const setActiveCountriesAtom = atom(null, async (_get, set) => {
  try {
    const response = await GetActiveCountries();
    set(activeCountriesAtom, response);
  } catch (error) {
    console.error("Failed to fetch active countries:", error);
    message.error(t`Failed to fetch countries`);
    set(activeCountriesAtom, []);
  }
});

export const toggleCountryActiveAtom = atom(
  null,
  async (get, set, { code, active }: { code: string; active: boolean }) => {
    const previous = get(activeCountriesAtom);
    // Optimistic update — this is a per-row Switch, not a form submit.
    set(
      activeCountriesAtom,
      active ? [...previous, code].sort() : previous.filter((c) => c !== code),
    );
    try {
      await SetCountryActive(code, active);
    } catch (error) {
      console.error("Failed to update country activation:", error);
      message.error(t`Failed to update country`);
      set(activeCountriesAtom, previous);
    }
  },
);
