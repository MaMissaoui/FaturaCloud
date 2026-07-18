import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import babel from "@rolldown/plugin-babel";
import { lingui } from "@lingui/vite-plugin";
import { sentryVitePlugin } from "@sentry/vite-plugin";

export default defineConfig(async () => ({
  plugins: [
    react(),
    await babel({
      plugins: ["@lingui/babel-plugin-lingui-macro"],
      presets: ["jotai-babel/preset"],
    }),
    lingui(),
    sentryVitePlugin({
      org: "mohamed-ali-missaoui",
      project: "faturacloud",
      telemetry: false,
      release: {
        // Matches the VERSION build-arg embedded in the Go binary and served by
        // GET /api/version — Sentry.init's `release` (src/utils/sentry.ts) reads
        // that same string at runtime, so uploaded source maps line up with
        // reported events. GITHUB_SHA never reaches the Docker build container,
        // so it was never a usable value here.
        name: process.env.VERSION || "development",
      },
      sourcemaps: {
        assets: "./dist/**",
        ignore: ["node_modules"],
        // Never ship source maps in the deployed artifact: the Go server embeds
        // all of dist/ (//go:embed all:dist), so any .map left here is served
        // publicly at /assets/*.js.map — full original source exposure plus
        // dead weight in the binary. The plugin uploads them to Sentry first
        // (when an auth token is present), then deletes them; with no token it
        // just deletes. Combined with build.sourcemap: "hidden" below (no
        // sourceMappingURL comment), maps exist only inside Sentry.
        filesToDeleteAfterUpload: ["./dist/**/*.map"],
      },
    }),
  ],
  optimizeDeps: {
    include: ["pdfjs-dist"],
  },
  css: {
    preprocessorOptions: {
      scss: {
        api: "modern-compiler",
      },
    },
  },
  resolve: {
    alias: [{ find: "src", replacement: "/src" }],
  },
  define: {
    global: "globalThis",
  },
  clearScreen: false,
  server: {
    port: 5173,
    host: "127.0.0.1",
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
      },
    },
  },
  envPrefix: ["VITE_"],
  build: {
    target: "es2020",
    minify: "esbuild",
    // "hidden" still generates maps (Sentry uploads them at build time) but
    // omits the //# sourceMappingURL comment, so even a stray .map is never
    // advertised to the browser. The Sentry plugin deletes them post-upload
    // (see sourcemaps.filesToDeleteAfterUpload) so none reach the dist/ the
    // Go binary embeds.
    sourcemap: "hidden",
    outDir: "dist",
  },
}));
