import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import path from "path";

// The build emits into the Go server's embed directory so the single binary
// serves the SPA via go:embed.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
    },
  },
  server: {
    proxy: {
      "/api": "http://localhost:8080",
    },
  },
  build: {
    outDir: path.resolve(__dirname, "../server/internal/web/dist"),
    emptyOutDir: true,
  },
});
