import { defineConfig } from "vitest/config";

// Deliberately separate from vite.config.ts rather than a shared `test:`
// block: that file's plugin pipeline (Lingui Babel macros, jotai-babel,
// Sentry source-map upload) exists for the real app build and none of it is
// needed to run pure-function unit tests — pulling it in would make test
// runs depend on Sentry auth/network state for no benefit. Only the `src`
// alias is shared, since the tests import from `src/...` the same way the
// app does.
export default defineConfig({
  resolve: {
    alias: [{ find: "src", replacement: "/src" }],
  },
});
