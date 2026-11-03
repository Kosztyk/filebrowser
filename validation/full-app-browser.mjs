import {
  chromium,
  expect,
} from "../frontend/node_modules/@playwright/test/index.mjs";
let input = "";
for await (const chunk of process.stdin) input += chunk;
const { base, username, password } = JSON.parse(input);
import assert from "node:assert/strict";
const browser = await chromium.launch({
  executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH,
  headless: true,
  args: ["--no-sandbox", "--no-zygote", "--single-process"],
});
const page = await browser.newPage({ viewport: { width: 1800, height: 1000 } });
const errors = [];
page.on("pageerror", (e) => errors.push(e.message));
const requests = [];
page.on("request", (r) => requests.push(r.url()));
async function api(path, method = "GET", body) {
  return page.evaluate(
    async ({ path, method, body }) => {
      const r = await fetch(path, {
        method,
        headers: {
          "X-Auth": localStorage.getItem("jwt"),
          "Content-Type": "application/json",
        },
        body: body ? JSON.stringify(body) : undefined,
      });
      return {
        status: r.status,
        body: await r.text(),
        headers: Object.fromEntries(r.headers),
      };
    },
    { path, method, body },
  );
}
async function job(id) {
  for (let i = 0; i < 100; i++) {
    const r = await api("/api/jobs/" + id);
    assert.equal(r.status, 200);
    const j = JSON.parse(r.body);
    if (j.state === "succeeded") return j;
    assert.ok(["queued", "running"].includes(j.state), j.message || j.state);
    await page.waitForTimeout(150);
  }
  throw new Error("job timeout");
}
try {
  await page.goto(base + "/login");
  await page.locator("input[type=text]").first().fill(username);
  await page.locator("input[type=password]").fill(password);
  await page.locator("input[type=submit],button[type=submit]").first().click();
  await page.waitForURL("**/files/**");
  await expect(page.getByText(/1–200 of \d+/, { exact: true })).toBeVisible();
  const cards = () => page.locator("#listing .item:not(.header)");
  assert.ok((await cards().count()) <= 200, "rendered more than a bounded page");
  assert.ok(!requests.some((u) => u.includes("vue-reader.es")), "EPUB loaded eagerly");
  await cards().first().click({ modifiers: ["Control"] });
  await expect(page.getByText("1 selected across pages", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Next page", exact: true }).click();
  await expect(page.getByText(/201–400 of \d+/, { exact: true })).toBeVisible();
  await cards().first().click({ modifiers: ["Control"] });
  await expect(page.getByText("2 selected across pages", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Previous page", exact: true }).click();
  await expect(page.getByText("2 selected across pages", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Clear selection", exact: true }).click();
  await expect(page.locator('#listing [aria-selected="true"]')).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Clear selection", exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Next page", exact: true }).click();
  await expect(page.getByText(/201–400 of \d+/, { exact: true })).toBeVisible();
  await expect(page.locator('#listing [aria-selected="true"]')).toHaveCount(0);
  const openedName = await cards().first().getAttribute("aria-label");
  await cards().first().dblclick();
  await page.waitForURL((url) => url.pathname.endsWith(openedName));
  await expect(page.locator(".ace_editor")).toBeVisible();
  await page.goBack();
  await expect(page.getByText(/201–400 of \d+/, { exact: true })).toBeVisible();
  await expect(page.locator('#listing [aria-selected="true"]')).toHaveAttribute("aria-label", openedName);
  console.log("PASS bounded pages, cross-page selection, clear selection, return to the previous page and lazy EPUB loading");

  await page.goto(base + "/files/layout/");
  await expect(page.locator(".directory-count")).toHaveText("45 items");
  await expect(page.locator("#listing > h2")).toHaveText(["Folders", "Files"]);
  await expect(cards()).toHaveCount(45);
  assert.deepEqual(await cards().evaluateAll((items) => items.map((el) => el.dataset.dir)), [...Array(29).fill("true"), ...Array(16).fill("false")]);
  const view = page.locator("header").getByRole("button", { name: "Switch view", exact: true });
  if ((await page.locator("#listing").getAttribute("class")).includes("list")) await view.click();
  await expect(page.locator("#listing")).toHaveClass(/mosaic/);
  const style = await cards().first().evaluate((el) => {
    const s = getComputedStyle(el); return { margin: parseFloat(s.marginLeft), shadow: s.boxShadow, height: el.clientHeight };
  });
  assert.ok(style.margin > 0 && style.shadow !== "none", "original card spacing and shadow missing");
  assert.ok(style.height < 150, "card height overridden by virtual layout");
  assert.equal(await page.locator(".virtual-directory").count(), 0);
  await expect(page.getByRole("button", { name: "Next page", exact: true })).toHaveCount(0);
  await page.locator(".directory-sort").selectOption("name:true");
  await expect(cards().first()).toHaveAttribute("aria-label", "folder-28");
  await expect(page.locator('#listing .item[data-dir="false"]').first()).toHaveAttribute("aria-label", "file-14.txt");
  await page.locator(".directory-sort").selectOption("name:false");
  await expect(cards().first()).toHaveAttribute("aria-label", "folder-00");
  await page.locator('#listing [aria-label="file-04.txt"]').click({ modifiers: ["Control"] });
  const filter = page.getByRole("textbox", { name: "Filter names", exact: true });
  await filter.click(); await page.keyboard.press("Control+a"); await page.keyboard.press("Delete");
  await expect(page.getByText("1 selected", { exact: true })).toBeVisible();
  await expect(page.locator(".card.floating")).toHaveCount(0);
  await filter.fill("file-05");
  await expect(page.locator(".directory-count")).toHaveText("1 item");
  await expect(cards()).toHaveCount(1);
  await expect(cards().first()).toHaveAttribute("aria-label", "file-05.txt");
  await expect(page.locator(".directory-selection")).toHaveCount(0);
  await page.locator(".directory-sort").selectOption("size:false");
  await expect(filter).toHaveValue("file-05");
  await page.getByRole("button", { name: "Clear filter", exact: true }).click();
  await expect(page.locator(".directory-count")).toHaveText("45 items");
  await page.locator(".directory-sort").selectOption("name:false");
  await expect(cards().first()).toHaveAttribute("aria-label", "folder-00");
  console.log("PASS original folder/file sections, cards, immediate sorting, live filtering and clear filter");

  const file = page.locator('#listing [aria-label="file-03.txt"]');
  await file.click({ button: "right" });
  const menu = page.locator(".context-menu");
  await expect(menu).toBeVisible();
  for (const label of ["Rename", "Copy file", "Move file", "Delete", "Download", "Info"])
    await expect(menu.getByRole("button", { name: label, exact: true })).toBeVisible();
  await menu.getByRole("button", { name: "Rename", exact: true }).click();
  await page.locator("#focus-prompt").fill("renamed-file.txt");
  await page.locator(".card.floating").getByRole("button", { name: "Rename", exact: true }).click();
  await expect(page.locator('#listing [aria-label="renamed-file.txt"]')).toBeVisible();
  await page.getByRole("button", { name: "Clear selection", exact: true }).click();
  await page.locator('#listing [aria-label="drag-source.txt"]').dragTo(page.locator('#listing [aria-label="folder-00"]'));
  await expect(page.locator('#listing [aria-label="drag-source.txt"]')).toHaveCount(0);
  assert.equal((await api("/api/resources/layout/folder-00/drag-source.txt")).status, 200);
  await expect(page.locator('#listing [aria-label="folder-01"]')).toBeVisible();
  // Native file drops use the public item URL, including when dropping onto an icon child.
  await page.evaluate(() => {
    const data = new DataTransfer(); data.items.add(new File(["drop fixture"], "dropped.txt", { type: "text/plain" }));
    document.querySelector('#listing [aria-label="folder-01"] i').dispatchEvent(new DragEvent("drop", { bubbles: true, cancelable: true, dataTransfer: data }));
  });
  await expect.poll(async () => (await api("/api/resources/layout/folder-01/dropped.txt")).status).toBe(200);
  console.log("PASS right-click actions, rename, internal drag/move and native file drop into a folder");

  await view.click();
  await expect(page.locator("#listing")).toHaveClass(/gallery/);
  await view.click();
  await expect(page.locator("#listing")).toHaveClass(/list/);
  const nameHeader = page.locator(".item.header .name");
  await nameHeader.click();
  await expect(cards().first()).toHaveAttribute("aria-label", "folder-28");
  await expect(page.locator(".directory-sort")).toHaveValue("name:true");
  await nameHeader.focus(); await page.keyboard.press("Enter");
  await expect(cards().first()).toHaveAttribute("aria-label", "folder-00");
  await view.click();
  await expect(page.locator("#listing")).toHaveClass(/mosaic/);
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(filter).toBeVisible();
  assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1), "mobile layout overflows horizontally");
  await page.locator('#listing [aria-label="folder-00"]').click({ button: "right" });
  await expect(menu).toBeVisible();
  const bounds = await menu.boundingBox();
  assert.ok(bounds.x >= 0 && bounds.x + bounds.width <= 391 && bounds.y + bounds.height <= 845, "context menu outside mobile viewport");
  await page.keyboard.press("Escape");
  await page.setViewportSize({ width: 2000, height: 1100 });
  await expect.poll(() => page.locator(".directory-sort").evaluate((el) => getComputedStyle(el).order)).toBe("0");
  if (process.env.FILEBROWSER_UI_SCREENSHOT) await page.screenshot({ path: process.env.FILEBROWSER_UI_SCREENSHOT });
  console.log("PASS gallery/list/mosaic modes, immediate column sorting, keyboard sorting and mobile layout");
  await page.goto(base + "/files/large.csv");
  await expect(
    page.getByText("Read-only preview:", { exact: false }),
  ).toBeVisible();
  await expect(page.locator(".bounded-text table tr").first()).toBeVisible();
  assert.ok((await page.locator(".bounded-text table tr").count()) <= 2000);
  await page.getByRole("button", { name: "Next", exact: true }).click();
  await expect(page.locator(".bounded-text")).toContainText("record-1999");
  assert.equal(
    requests.filter((u) => u.includes("/api/resources/large.csv")).length,
    1,
    "large CSV downloaded through an unbounded resource request",
  );
  console.log("PASS bounded CSV paging and metadata-only loading");
  await page.goto(base + "/files/large.log");
  await expect(page.locator(".bounded-text pre")).toContainText(
    "Bounded log fixture",
  );
  assert.ok(
    (await page.locator(".bounded-text pre").innerText()).length <= 262144,
  );
  console.log("PASS bounded log window");
  await page.goto(base + "/files/image.tiff");
  await page.waitForTimeout(500);
  const hd = page
    .locator("button")
    .filter({ has: page.locator("i", { hasText: /^hd$/ }) });
  if (await hd.count()) await hd.click();
  await expect
    .poll(
      () => page.locator(".image-ex-img").evaluate((img) => img.naturalWidth),
      { timeout: 20000 },
    )
    .toBe(2);
  assert.ok(
    requests.some((u) => u.includes("imageDecode-")),
    "image worker not loaded",
  );
  console.log("PASS TIFF decoded in a worker");
  const historyName = "browser-history-" + Date.now() + ".txt";
  let r = await api("/api/resources/" + historyName, "POST");
  assert.ok([200, 201].includes(r.status), r.body);
  r = await api("/api/resources/" + historyName, "PUT", "first");
  assert.ok(r.status < 300, r.body);
  r = await api("/api/resources/" + historyName, "PUT", "second");
  assert.ok(r.status < 300, r.body);
  r = await api("/api/history/" + historyName);
  assert.equal(r.status, 200);
  assert.ok(JSON.parse(r.body).length >= 1);
  console.log("PASS previous contents API");
  r = await api("/api/downloads", "POST", {
    items: ["/folder"],
    name: "folder.zip",
  });
  assert.equal(r.status, 202);
  const j = await job(JSON.parse(r.body).id);
  r = await page.evaluate(async (id) => {
    const r = await fetch("/api/downloads/" + id, {
      headers: { "X-Auth": localStorage.getItem("jwt"), Range: "bytes=0-9" },
    });
    return {
      status: r.status,
      length: (await r.arrayBuffer()).byteLength,
      etag: r.headers.get("etag"),
    };
  }, j.id);
  assert.equal(r.status, 206);
  assert.equal(r.length, 10);
  assert.ok(r.etag);
  console.log("PASS durable prepared ZIP, range download and ETag");
  await page.goto(base + "/settings/system");
  await expect(
    page.getByRole("heading", { name: "About / System", exact: true }),
  ).toBeVisible();
  await expect(page.locator("dl").first()).toContainText("3.3.1");
  if (process.env.FILEBROWSER_EXPECTED_SOURCE_SHA) await expect(page.locator("dl").first()).toContainText(process.env.FILEBROWSER_EXPECTED_SOURCE_SHA);
  await expect(
    page.getByRole("button", { name: "Revoke", exact: true }).first(),
  ).toBeVisible();
  await expect(
    page.getByText("Reported version:", { exact: false }).first(),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Save limits and timeouts", exact: true }),
  ).toBeVisible();
  console.log("PASS About/System, sessions, diagnostics and limits UI");
  assert.deepEqual(errors, []);
  console.log("PASS no browser runtime errors");
} catch (error) {
  if (process.env.FILEBROWSER_UI_SCREENSHOT) await page.screenshot({ path: process.env.FILEBROWSER_UI_SCREENSHOT });
  throw error;
} finally {
  await browser.close();
}
