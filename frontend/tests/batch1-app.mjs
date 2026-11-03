import { chromium, expect } from "@playwright/test";
import { writeFile } from "node:fs/promises";
import { join } from "node:path";
let input = "";
for await (const c of process.stdin) input += c;
const { base, username, password, temporary } = JSON.parse(input);
const browser = await chromium.launch({
  executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH,
  args: JSON.parse(process.env.FILEBROWSER_BROWSER_ARGS || "[]"),
});
try {
  const page = await browser.newPage(),
    errors = [];
  page.on("dialog", (dialog) => dialog.accept());
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto(base + "/login");
  await page.locator('input[type="text"]').fill(username);
  await page.locator('input[type="password"]').fill(password);
  await page.locator('input[type="submit"],button[type="submit"]').click();
  await expect(
    page.getByText("upgrade-transfer.bin", { exact: true }).first()
  ).toBeVisible();
  let holdAt = 1,
    patches = 0,
    release,
    blocked;
  const setBlock = () =>
    new Promise((r) => {
      blocked = r;
    });
  let waiting = setBlock();
  await page.route("**/api/uploads/**", async (route) => {
    if (route.request().method() === "PATCH" && ++patches === holdAt) {
      blocked();
      await new Promise((r) => {
        release = r;
      });
    }
    await route.continue().catch(() => {});
  });
  const source = join(temporary, "ui-transfer.bin");
  await writeFile(source, Buffer.alloc(32 * 1024 * 1024, 91));
  await page.locator("#upload-input").first().setInputFiles(source);
  await waiting;
  let row = page
    .locator(".transfer-row")
    .filter({ hasText: "ui-transfer.bin" });
  await row.getByRole("button", { name: "Pause", exact: true }).click();
  release();
  holdAt = 0;
  await expect(row).toHaveAttribute("data-status", "paused");
  await row.getByRole("button", { name: "Resume", exact: true }).click();
  await expect(row).toHaveAttribute("data-status", "ready", { timeout: 60000 });
  await expect(row).toContainText("Scanning disabled");
  const recovery = join(temporary, "ui resume # ?.bin");
  await writeFile(recovery, Buffer.alloc(48 * 1024 * 1024, 42));
  holdAt = 2;
  patches = 0;
  waiting = setBlock();
  await page.locator("#upload-input").first().setInputFiles(recovery);
  await waiting;
  holdAt = 0;
  const resumeRelease = release;
  await page.reload();
  resumeRelease();
  row = page.locator(".transfer-row").filter({ hasText: "ui resume # ?.bin" });
  await expect(row).toHaveAttribute("data-status", "needs_file");
  await row.locator('input[type="file"]').setInputFiles(recovery);
  await expect(row).toHaveAttribute("data-status", "ready", { timeout: 60000 });
  const cancelled = join(temporary, "ui-cancel.bin");
  await writeFile(cancelled, Buffer.alloc(32 * 1024 * 1024, 13));
  holdAt = 1;
  patches = 0;
  waiting = setBlock();
  await page.locator("#upload-input").first().setInputFiles(cancelled);
  await waiting;
  row = page.locator(".transfer-row").filter({ hasText: "ui-cancel.bin" });
  await row.getByRole("button", { name: "Cancel", exact: true }).click();
  release();
  holdAt = 0;
  await expect(row).toHaveAttribute("data-status", "cancelled");
  await page
    .getByRole("button", { name: "Clear completed history", exact: true })
    .click();
  await expect(page.locator(".transfer-row")).toHaveCount(0);
  const jwt = await page.evaluate(() => localStorage.getItem("jwt"));
  for (const name of ["ui-transfer.bin", "ui resume # ?.bin"])
    expect(
      (
        await page.request.get(base + "/api/raw/" + encodeURIComponent(name), {
          headers: { "X-Auth": jwt },
        })
      ).status()
    ).toBe(200);
  expect(
    (
      await page.request.get(base + "/api/raw/ui-cancel.bin", {
        headers: { "X-Auth": jwt },
      })
    ).status()
  ).toBe(404);
  expect(errors).toEqual([]);
  console.log(
    JSON.stringify({
      production_browser: "pass",
      pause_resume: "pass",
      reload_reselect_resume: "pass",
      reserved_filename: "pass",
      cancel: "pass",
      clear_history_preserves_files: "pass",
      page_errors: errors,
    })
  );
} finally {
  await browser.close();
}
