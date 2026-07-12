import type { LinguiConfig } from "@lingui/conf";

const config: LinguiConfig = {
  locales: ["en", "en-GB", "de", "et", "fi", "fr", "el", "nl", "pt", "sv", "uk"],
  // Without this, `lingui extract` leaves the source-language catalog's
  // msgstr empty instead of auto-filling it with msgid, which is why en.po
  // had zero translated strings until this was added.
  sourceLocale: "en",
  catalogs: [
    {
      path: "<rootDir>/src/locales/{locale}",
      include: ["src"],
    },
  ],
};

export default config;
