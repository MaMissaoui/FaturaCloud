import "@testing-library/jest-dom/vitest";
import { afterEach } from "vitest";
import { cleanup } from "@testing-library/react";
import { i18n } from "@lingui/core";

// Unmount anything still mounted and flush React 19's scheduler before Vitest
// tears the jsdom environment down. React schedules work through a
// MessageChannel callback; a component whose async effect resolves just after
// its test ends can leave one queued, and it then fires with `window` already
// gone — surfacing as an *unhandled* "ReferenceError: window is not defined"
// that fails the whole run even though every test passed (seen on CI, where
// the extra load makes the race land). Doing it globally also covers the files
// that never called cleanup() themselves.
afterEach(async () => {
  cleanup();
  await new Promise((resolve) => setTimeout(resolve, 0));
});

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
