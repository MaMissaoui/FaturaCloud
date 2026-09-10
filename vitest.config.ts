import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import babel from "@rolldown/plugin-babel";

// Deliberately separate from vite.config.ts rather than a shared `test:`
// block: that file's plugin pipeline still carries things a test run has no
// use for (Sentry source-map upload, the Lingui Vite plugin's .po-catalog
// compilation) — pulling those in would make test runs depend on Sentry
// auth/network state, or on compiled catalogs being present, for no benefit.
//
// react() + the Lingui Babel macro plugin (issue #175) ARE needed here,
// though: any component under test that imports `Trans`/`t` from
// `@lingui/react/macro` / `@lingui/core/macro` only compiles through that
// transform — those packages have no real runtime export on their own.
// jotai-babel/preset is included for parity with the app build (atom debug
// labels), not because a test currently depends on it.
export default defineConfig(async () => ({
  plugins: [
    react(),
    await babel({
      plugins: ["@lingui/babel-plugin-lingui-macro"],
      presets: ["jotai-babel/preset"],
    }),
  ],
  resolve: {
    alias: [{ find: "src", replacement: "/src" }],
  },
  test: {
    // Global jsdom is safe for the existing pure-function tests
    // (currency.test.ts/invoice.test.ts touch no DOM/node APIs) and is what
    // every component test needs — no per-file environment split required.
    environment: "jsdom",
    setupFiles: ["./vitest.setup.ts"],
  },
}));
