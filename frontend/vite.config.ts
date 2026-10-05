import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import wails from "@wailsio/runtime/plugins/vite";

export default defineConfig({
  // wails3 dev starts this server and sets the port (Taskfile.yml).
  server: { host: "127.0.0.1", port: Number(process.env.WAILS_VITE_PORT) || 9245, strictPort: true },
  plugins: [react(), tailwindcss(), wails("./bindings")],
  resolve: { alias: { "@": path.resolve(import.meta.dirname, "./src") } },
  build: { target: "es2022" },
});
