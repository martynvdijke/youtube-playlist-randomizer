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
    await expect(quotaText).toContainText("Quota:");
  });

  test("shows loading state initially", async ({ page }) => {
    await page.goto("/");
    const loading = page.locator("#loading");
    await expect(loading).toContainText("Loading playlists");
  });

  test("randomize modal is hidden by default", async ({ page }) => {
    await page.goto("/");
    const modal = page.locator("#modal");
    await expect(modal).toHaveClass(/hidden/);
    await expect(modal).not.toBeVisible();
  });

  test("shows no-playlists message when API returns empty", async ({ page }) => {
    await page.goto("/");
    const noPlaylists = page.locator("#no-playlists");
    await expect(noPlaylists).not.toHaveClass(/hidden/);
    await expect(noPlaylists).toContainText("No playlists found");
  });
});
