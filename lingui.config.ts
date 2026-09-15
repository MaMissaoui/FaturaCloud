import type { LinguiConfig } from "@lingui/conf";

const config: LinguiConfig = {
  locales: ["en", "de", "fr"],
  // Without this, `lingui extract` leaves the source-language catalog's
  // msgstr empty instead of auto-filling it with msgid, which is why en.po
  // had zero translated strings until this was added.
  sourceLocale: "en",
  catalogs: [
    {
      path: "<rootDir>/src/locales/{locale}",
      include: ["src"],
      // Test files use the Lingui macros to assert the macros themselves
      // work (src/test-support/lingui-macros.test.tsx), so without this
      // their fixture strings ("hello", "world") land in every catalog as
      // untranslated UI text (audit 2026-09-14 F92). Excluding them also
      // keeps the CI catalog-drift gate from flagging churn nobody can
      // meaningfully translate.
      exclude: ["**/*.test.ts", "**/*.test.tsx", "**/test-support/**"],
    },
  ],
};

export default config;
