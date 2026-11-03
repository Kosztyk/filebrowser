import { defineConfig, devices } from "@playwright/test";

export default defineConfig({
  testDir: "./tests/browser",
  fullyParallel: false,
  use: {
    baseURL: "http://127.0.0.1:5175",
    trace: "retain-on-failure",
    launchOptions: {
      executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH,
      args: JSON.parse(process.env.FILEBROWSER_BROWSER_ARGS || "[]"),
    },
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: {
    command: "pnpm exec vite --host 127.0.0.1 --port 5175 --strictPort",
    url: "http://127.0.0.1:5175/tests/browser/index.html",
    reuseExistingServer: !process.env.CI,
  },
});
