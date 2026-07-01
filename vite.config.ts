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
      org: "konstruktor",
      project: "fatura-cloud",
      telemetry: false,
      release: {
        name: process.env.GITHUB_SHA || "development",
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
