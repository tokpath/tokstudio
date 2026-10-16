import { defineConfig } from "@playwright/test";
const port = process.env.TOKENHUB_WEB_TEST_PORT || "3003";
export default defineConfig({
  testDir: "./e2e", fullyParallel: false, workers: 1,
  use: { baseURL: process.env.TOKENHUB_WEB_ORIGIN || `http://127.0.0.1:${port}`, channel: process.env.PLAYWRIGHT_BROWSER_CHANNEL || undefined, locale: "zh-CN", viewport: { width: 1365, height: 960 } },
  webServer: process.env.PLAYWRIGHT_SKIP_WEBSERVER ? undefined : { command: `"${process.execPath}" node_modules/next/dist/bin/next start -p ${port}`, url: `http://127.0.0.1:${port}`, reuseExistingServer: false, timeout: 120_000 },
});
