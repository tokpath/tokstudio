import path from "node:path";
import { defineConfig } from "vitest/config";

export default defineConfig({
  resolve: {
    alias: { "@": path.resolve(__dirname, ".") },
  },
  esbuild: { jsx: "automatic" },
  test: {
    environment: "node",
    exclude: ["e2e/**", "node_modules/**", ".next/**"],
  },
});
