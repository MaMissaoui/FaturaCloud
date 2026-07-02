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
