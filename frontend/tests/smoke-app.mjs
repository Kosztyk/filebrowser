import { chromium, expect } from "@playwright/test";

let input = "";
for await (const chunk of process.stdin) input += chunk;
const { base, username, password } = JSON.parse(input);
const browser = await chromium.launch({
  executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH,
});
try {
  const page = await browser.newPage();
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(`${base}/login`);
  await page.locator('input[type="text"]').fill(username);
  await page.locator('input[type="password"]').fill(password);
  await page.locator('input[type="submit"],button[type="submit"]').click();
  await expect(
    page.getByText("upgrade-transfer.bin", { exact: true }).first()
  ).toBeVisible();
  expect(errors).toEqual([]);
  console.log("Production browser login and file listing: pass");
} finally {
  await browser.close();
}
