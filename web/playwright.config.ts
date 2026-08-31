import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./e2e",
  use: {
    baseURL: process.env.TOKENHUB_WEB_ORIGIN || "http://127.0.0.1:3000",
    extraHTTPHeaders: { "Accept-Language": "zh-CN,zh;q=0.9" },
  },
  webServer: process.env.PLAYWRIGHT_SKIP_WEBSERVER
    ? undefined
    : {
        command: "npm run start",
        url: "http://127.0.0.1:3000",
        reuseExistingServer: true,
      },
});
