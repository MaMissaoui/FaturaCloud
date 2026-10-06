/// <reference types="vite/client" />

import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [
    react({
      babel: {
        presets: ["jotai/babel/preset"],
      },
    }),
  ],
});

declare global {
  // Defined by vite.config.ts from the VERSION build-arg.
  const __APP_VERSION__: string;
}
