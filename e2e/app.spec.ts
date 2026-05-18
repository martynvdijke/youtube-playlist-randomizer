import { test, expect } from "@playwright/test";

test.describe("YouTube Playlist Randomizer", () => {
  test("page loads and shows the correct title", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator("h1")).toHaveText("YouTube Playlist Randomizer");
  });

  test("quota bar is visible on load", async ({ page }) => {
    await page.goto("/");
    const quotaBar = page.locator("#quota-bar");
    await expect(quotaBar).toBeVisible();
    await expect(quotaBar).toContainText("Quota:");
  });

  test("quota bar refreshes via htmx", async ({ page }) => {
    await page.goto("/");
    const quotaBar = page.locator("#quota-bar");
    await expect(quotaBar).toBeVisible();
    await expect(quotaBar).toContainText("used");
  });

  test("playlists load via htmx", async ({ page }) => {
    await page.goto("/");
    const playlistList = page.locator("#playlist-list");
    await expect(playlistList).toContainText("No playlists found", { timeout: 5000 });
  });

  test("randomize modal appears on button click and submits via htmx", async ({ page }) => {
    await page.goto("/");
    const modal = page.locator("#modal");
    await expect(modal).not.toBeVisible();
  });

  test("search input filters playlists via htmx", async ({ page }) => {
    await page.goto("/");
    const searchInput = page.locator("#search");
    await expect(searchInput).toBeVisible();
    await searchInput.fill("test");
    const playlistList = page.locator("#playlist-list");
    await expect(playlistList).toBeVisible({ timeout: 5000 });
  });

  test("progress modal is hidden on load", async ({ page }) => {
    await page.goto("/");
    const progressModal = page.locator("#progress-modal");
    await expect(progressModal).not.toBeVisible();
  });

  test("page has htmx headers", async ({ page }) => {
    await page.goto("/");
    const script = page.locator('script[src*="htmx"]');
    await expect(script).toHaveCount(1);
  });

  test("static assets load correctly", async ({ page }) => {
    const response = await page.goto("/static/style.css");
    expect(response?.status()).toBe(200);
  });

  test("API quota endpoint returns JSON", async ({ page }) => {
    const response = await page.request.get("/api/quota");
    expect(response.status()).toBe(200);
    const body = await response.json();
    expect(body).toHaveProperty("used");
    expect(body).toHaveProperty("limit");
    expect(body).toHaveProperty("remaining");
    expect(body).toHaveProperty("date");
  });

  test("API playlists endpoint returns JSON in mock mode", async ({ page }) => {
    const response = await page.request.get("/api/playlists");
    expect(response.status()).toBe(200);
    const body = await response.json();
    expect(Array.isArray(body)).toBe(true);
    expect(body.length).toBe(0);
  });

  test("API quota HTML endpoint returns HTML fragment", async ({ page }) => {
    const response = await page.request.get("/api/quota/html");
    expect(response.status()).toBe(200);
    const text = await response.text();
    expect(text).toContain("quota");
    expect(text).toContain("quota-fill");
  });

  test("API playlists HTML endpoint returns HTML fragment", async ({ page }) => {
    const response = await page.request.get("/api/playlists/html");
    expect(response.status()).toBe(200);
    const text = await response.text();
    expect(text).toContain("No playlists found");
  });

  test("API randomize returns error in mock mode", async ({ page }) => {
    const response = await page.request.post("/api/randomize", {
      data: { playlistId: "PL123", newName: "Test" },
    });
    expect(response.status()).toBe(400);
    const body = await response.json();
    expect(body.error).toContain("mock mode");
  });

  test("API randomize HTML returns error in mock mode", async ({ page }) => {
    const response = await page.request.post("/api/randomize/html", {
      form: { playlistId: "PL123", newName: "Test" },
    });
    expect(response.status()).toBe(400);
    const text = await response.text();
    expect(text).toContain("mock mode");
  });

  test("API modal HTML returns error without required params", async ({ page }) => {
    const response = await page.request.get("/api/modal/html");
    expect(response.status()).toBe(200);
    const text = await response.text();
    expect(text).toContain("Randomize");
  });

  test("unknown route returns 404", async ({ page }) => {
    const response = await page.goto("/nonexistent");
    expect(response?.status()).toBe(404);
  });

  test("job status on unknown job returns 404", async ({ page }) => {
    const response = await page.request.get("/api/jobs/nonexistent");
    expect(response.status()).toBe(404);
  });

  test("index page has correct structure", async ({ page }) => {
    await page.goto("/");
    await expect(page.locator("header")).toBeVisible();
    await expect(page.locator("main")).toBeVisible();
    await expect(page.locator("#playlists")).toBeVisible();
    await expect(page.locator("#search")).toBeVisible();
  });

  test("search input has correct htmx attributes", async ({ page }) => {
    await page.goto("/");
    const search = page.locator("#search");
    await expect(search).toHaveAttribute("hx-get", "/api/playlists/html");
    await expect(search).toHaveAttribute("hx-target", "#playlist-list");
  });
});
