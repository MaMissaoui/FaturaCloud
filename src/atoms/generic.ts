import { atomWithStorage } from "jotai/utils";
import { defaultLocale } from "src/utils/lingui";

// Generic UI state atoms
export const siderAtom = atomWithStorage("sider", false);
siderAtom.debugLabel = "siderAtom";

export const localeAtom = atomWithStorage("locale", defaultLocale);
localeAtom.debugLabel = "localeAtom";

// Color theme: "light" | "dark", switchable at runtime, persisted
export const themeAtom = atomWithStorage<"light" | "dark">("theme", "light");
themeAtom.debugLabel = "themeAtom";

// Which Cash Book layout this browser shows: "v1" (the stacked screen) or
// "v2" (counter + customer-loans tabs), kept side by side until one is
// chosen. Defaults to v1 so no till changes until someone opts in.
export type CashBookLayout = "v1" | "v2";
export const cashBookLayoutAtom = atomWithStorage<CashBookLayout>("cashBookLayout", "v1");
cashBookLayoutAtom.debugLabel = "cashBookLayoutAtom";
