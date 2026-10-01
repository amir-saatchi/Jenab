import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Each scenario (and each Markdown renderer) is a dynamic import, so the bundler
// splits chunks naturally; the runner walks the static-import closure of each
// entry chunk to report the size a feature adds.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  resolve: { alias: { "@": path.resolve(import.meta.dirname, "./src") } },
  build: {
    target: "es2022",
    chunkSizeWarningLimit: 5000,
    rollupOptions: { external: ["/wails/runtime.js"] },
  },
});
