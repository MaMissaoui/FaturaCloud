import "@testing-library/jest-dom/vitest";
import { i18n } from "@lingui/core";

// Real app startup (src/utils/lingui.tsx) does this same synchronous
// load+activate before loading a locale's actual .po catalog — with no
// catalog loaded, Trans/t render their source-locale (English) text
// unchanged, which is exactly what a component test wants to assert
// against rather than depending on translated strings.
i18n.load("en", {});
i18n.activate("en");

// jsdom has no window.matchMedia — antd's responsive components (Grid,
// theme algorithm's dark-mode detection, etc.) call it unconditionally on
// mount, so without a stub every render throws before it gets anywhere near
// the component under test.
if (typeof window !== "undefined" && !window.matchMedia) {
  window.matchMedia = (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  });
}

// jsdom has no ResizeObserver either — rc-resize-observer (a transitive
// dependency of antd's Table/Select/Form among others) instantiates one
// unconditionally on mount, same failure-before-the-test-even-starts shape
// as matchMedia above. A no-op stub is fine for jsdom, which never reports
// real size changes anyway.
if (typeof window !== "undefined" && !window.ResizeObserver) {
  window.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  };
}
