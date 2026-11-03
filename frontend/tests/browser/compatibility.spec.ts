import { expect, test } from "@playwright/test";

test("Video.js 10 plays media and exposes captions, speed and fullscreen controls", async ({
  page,
}) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/tests/browser/index.html");
  const video = page.locator("video");
  await expect
    .poll(() =>
      video.evaluate((element: HTMLVideoElement) => element.readyState)
    )
    .toBeGreaterThanOrEqual(2);
  await expect(
    page.getByRole("button", { name: "Play", exact: true }).first()
  ).toBeVisible();
  await page.getByRole("button", { name: "Play", exact: true }).first().click();
  await expect
    .poll(() =>
      video.evaluate((element: HTMLVideoElement) => element.currentTime)
    )
    .toBeGreaterThan(0);
  await expect
    .poll(() =>
      video.evaluate((element: HTMLVideoElement) => element.textTracks.length)
    )
    .toBe(1);
  await page.keyboard.press("k");
  await expect
    .poll(() => video.evaluate((element: HTMLVideoElement) => element.paused))
    .toBe(true);
  const initialRate = await video.evaluate(
    (element: HTMLVideoElement) => element.playbackRate
  );
  for (let i = 0; i < 6; i++) {
    await page.keyboard.press("Shift+Period");
  }
  await expect
    .poll(() =>
      video.evaluate((element: HTMLVideoElement) => element.playbackRate)
    )
    .toBeGreaterThan(initialRate);
  const fasterRate = await video.evaluate(
    (element: HTMLVideoElement) => element.playbackRate
  );
  await page.keyboard.press("Shift+Comma");
  await expect
    .poll(() =>
      video.evaluate((element: HTMLVideoElement) => element.playbackRate)
    )
    .toBeLessThan(fasterRate);
  await page.keyboard.press("ArrowRight");
  await expect
    .poll(() =>
      video.evaluate((element: HTMLVideoElement) => element.currentTime)
    )
    .toBeGreaterThan(9);
  await expect
    .poll(() =>
      video.evaluate((element: HTMLVideoElement) => element.textTracks[0].mode)
    )
    .toBe("showing");
  await page.keyboard.press("c");
  await expect
    .poll(() =>
      video.evaluate((element: HTMLVideoElement) => element.textTracks[0].mode)
    )
    .toBe("disabled");
  await expect(
    page.getByRole("button", { name: /full.?screen/i })
  ).toBeVisible();
  await expect(
    page
      .getByRole("button", { name: /playback.*speed|speed|settings/i })
      .first()
  ).toBeVisible();
  expect(errors).toEqual([]);
});

test("Vue Reader renders a zipped EPUB and reports its CFI location", async ({
  page,
}) => {
  await page.goto("/tests/browser/index.html?mode=epub");
  await expect(
    page.frameLocator("iframe").getByRole("heading", { name: "Chapter One" })
  ).toBeVisible();
  await expect
    .poll(() => page.locator("body").getAttribute("data-location"))
    .toMatch(/^epubcfi\(/);
});

test("browser math preview is sanitized and retains MathML", async ({
  page,
}) => {
  await page.goto("/tests/browser/index.html?mode=markdown");
  await expect(page.locator("#markdown math")).toHaveCount(1);
  await expect(page.locator("#markdown [onerror]")).toHaveCount(0);
});
