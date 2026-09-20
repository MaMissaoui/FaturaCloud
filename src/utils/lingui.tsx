import { i18n } from "@lingui/core";
import dayjs from "dayjs";
import config from "../../lingui.config";

export const locales = config.locales;

export const defaultLocale = "en";

// Initialize i18n synchronously with empty messages to prevent race conditions
// This ensures i18n.activate() is called before any translation functions
i18n.load(defaultLocale, {});
i18n.activate(defaultLocale);

// Load actual messages asynchronously and update
(async () => {
  try {
    const { messages } = await import(`../locales/${defaultLocale}.po`);
    i18n.load(defaultLocale, messages);
    i18n.activate(defaultLocale);
  } catch (error) {
    console.warn(`Failed to load default messages:`, error);
  }
})();

export async function dynamicActivate(locale: string) {
  // Fall back to the default if an unsupported locale is requested — e.g. a
  // value persisted in localStorage from a locale that has since been removed,
  // which would otherwise fail the dynamic .po import below.
  if (!locales.includes(locale)) {
    locale = defaultLocale;
  }

  const { messages } = await import(`../locales/${locale}.po`);
  i18n.load(locale, messages);
  i18n.activate(locale);

  dayjs.locale(locale);

  // F127: keep the document's declared language in sync with the active locale
  // so assistive tech and the browser's own translation/spellcheck logic see
  // the right language. index.html ships lang="en" as the pre-hydration
  // default; this updates it on every switch (both the app-mount effect in
  // src/app.tsx and the header language switcher in src/layouts/base.tsx go
  // through here). Guarded for non-DOM contexts (tests).
  if (typeof document !== "undefined") {
    document.documentElement.lang = locale;
  }
}
