import { test, expect } from "@playwright/test";

test.describe("YouTube Playlist Randomizer", () => {
  test("page loads and shows the correct title", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator("h1")).toHaveText("YouTube Playlist Randomizer");
  });

  test("quota bar is visible on load", async ({ page }) => {
    await page.goto("/");
    const quotaText = page.locator("#quota-text");
    await expect(quotaText).toBeVisible();
  });

  test("shows loading state initially", async ({ page }) => {
    await page.goto("/");
    const loading = page.locator("#loading");
    await expect(loading).toContainText("Loading playlists");
  });

  test("shows randomize modal and can close it", async ({ page }) => {
    await page.goto("/");
    const modal = page.locator("#modal");
    await expect(modal).toHaveClass(/hidden/);

    const cancelBtn = page.locator("#cancel-btn");
    await expect(cancelBtn).toBeVisible();
  });
});
