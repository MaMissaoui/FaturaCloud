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
    sourcemap: true,
    outDir: "dist",
  },
}));
