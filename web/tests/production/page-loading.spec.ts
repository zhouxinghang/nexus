import { test, expect, type Page } from "@playwright/test";
import { readdir, readFile } from "node:fs/promises";
import { gunzipSync } from "node:zlib";

const build = new URL("../../../server/internal/web/dist/", import.meta.url);
const toolChunk = /\/assets\/(?:Files|Servers|Tasks|Journal|Bookmarks|Overview)-[^/]+\.js$/;

async function mockAPI(page: Page, authenticated = true) {
  await page.route("**/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/auth/me") return route.fulfill({ status: authenticated ? 200 : 401, json: { authenticated } });
    if (path === "/api/auth/login") return route.fulfill({ json: { ok: true } });
    if (path === "/api/overview") return route.fulfill({ json: {
      host: { available: false, flags: {} },
      tasks: { todo: 0, doing: 0, done: 0, overdue: 0, due_today: 0 },
      journal: [], bookmarks: { up: 0, down: 0, unknown: 0, down_list: [] }, files: { total: 0, missing: 0 },
    } });
    if (path === "/api/files/resources") return route.fulfill({ json: { items: [], directories: [], total: 0, kinds: {}, limit: 200, offset: 0 } });
    if (path === "/api/journal/entries") return route.fulfill({ json: { items: [], next_cursor: null } });
    if (path === "/api/servers/current") return route.fulfill({ json: {
      cpu_percent: 20, cpu_cores: 8, mem_total: 16 * 1024 ** 3, mem_used: 8 * 1024 ** 3,
      disk_total: 512 * 1024 ** 3, disk_used: 100 * 1024 ** 3, disks: [], disk_rate: null,
      net_rx: 0, net_tx: 0, net_rate: null, load1: 1, load5: 1, load15: 1,
      pressure: null, uptime_secs: 3600, self: null, top_procs: [], docker: { enabled: false, containers: [] },
    } });
    return route.fulfill({ json: [] });
  });
}

test("login renders without waiting for any external font service", async ({ page }) => {
  const external: string[] = [];
  const origin = new URL(test.info().project.use.baseURL!).origin;
  await page.route("**/*", (route) => {
    if (new URL(route.request().url()).origin === origin) return route.fallback();
    external.push(new URL(route.request().url()).hostname);
    // An unavailable font service must not enter the initial rendering path.
    return new Promise<void>(() => {});
  });
  await mockAPI(page, false);
  await page.goto("/login", { waitUntil: "commit" });
  await expect(page.getByRole("heading", { name: "Welcome to Nexus" })).toBeVisible({ timeout: 1000 });
  expect(external).toEqual([]);
  const family = await page.evaluate(() => getComputedStyle(document.body).fontFamily);
  expect(family).toContain("system-ui");
  expect(family).not.toContain("Inter");
});

test("login does not fetch tool code and stays within the initial JS budget", async ({ page }) => {
  const scripts: string[] = [];
  page.on("request", (request) => {
    if (request.resourceType() === "script") scripts.push(new URL(request.url()).pathname);
  });
  await mockAPI(page, false);
  await page.goto("/login");
  await expect(page.getByRole("button", { name: "Sign in", exact: true })).toBeVisible();
  expect(scripts.filter((path) => toolChunk.test(path))).toEqual([]);
  const bytes = await page.evaluate(() => performance.getEntriesByType("resource")
    .filter((entry) => new URL(entry.name).pathname.endsWith(".js"))
    .reduce((total, entry) => total + (entry as PerformanceResourceTiming).decodedBodySize, 0));
  expect(bytes).toBeLessThan(250 * 1024);
});

test("Overview loads only its own tool chunk; navigating to Files loads Files on demand", async ({ page }) => {
  const chunks: string[] = [];
  page.on("request", (request) => {
    const path = new URL(request.url()).pathname;
    if (toolChunk.test(path)) chunks.push(path);
  });
  await mockAPI(page);
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Overview", exact: true })).toBeVisible();
  expect(chunks).toHaveLength(1);
  expect(chunks[0]).toMatch(/\/Overview-/);
  await page.locator("nav").getByRole("link", { name: "Files", exact: false }).click();
  await expect(page.getByRole("heading", { name: "Files", exact: true })).toBeVisible();
  expect(chunks).toHaveLength(2);
  expect(chunks[1]).toMatch(/\/Files-/);
  await page.goBack();
  await expect(page.getByRole("heading", { name: "Overview", exact: true })).toBeVisible();
  expect(chunks).toHaveLength(2);
});

test("navigation stays usable and announces loading while a lazy chunk is pending", async ({ page }) => {
  let release!: () => void;
  const pending = new Promise<void>((resolve) => { release = resolve; });
  await page.route(/\/assets\/Files-[^/]+\.js$/, async (route) => {
    await pending;
    await route.continue();
  });
  await mockAPI(page);
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Overview", exact: true })).toBeVisible();
  await page.locator("nav").getByRole("link", { name: "Files", exact: false }).click();
  await expect(page.getByRole("status", { name: "Loading page" })).toBeVisible();
  await expect(page.locator("nav").getByRole("link", { name: "Servers", exact: false })).toBeVisible();
  release();
  await expect(page.getByRole("heading", { name: "Files", exact: true })).toBeVisible();
  await expect(page.getByRole("status", { name: "Loading page" })).toHaveCount(0);
});

test("a failed lazy chunk keeps navigation and can recover by reloading", async ({ page }) => {
  let failed = false;
  await page.route(/\/assets\/Files-[^/]+\.js$/, async (route) => {
    if (!failed) {
      failed = true;
      return route.fulfill({ status: 503, body: "temporarily unavailable" });
    }
    return route.continue();
  });
  await mockAPI(page);
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Overview", exact: true })).toBeVisible();
  await page.locator("nav").getByRole("link", { name: "Files", exact: false }).click();
  await expect(page.getByRole("alert")).toContainText("Could not load this page");
  await expect(page.locator("nav").getByRole("link", { name: "Journal", exact: false })).toBeVisible();
  // A different route resets only the failed content, not the persistent Shell.
  await page.locator("nav").getByRole("link", { name: "Journal", exact: false }).click();
  await expect(page.getByRole("heading", { name: "Journal", exact: true })).toBeVisible();
  await page.locator("nav").getByRole("link", { name: "Files", exact: false }).click();
  await expect(page.getByRole("button", { name: "Reload page", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Reload page", exact: true }).click();
  await expect(page.getByRole("heading", { name: "Files", exact: true })).toBeVisible();
  await expect(page.getByRole("alert")).toHaveCount(0);
});

for (const [path, heading] of [["/files", "Files"], ["/servers", "Servers"], ["/tasks", "Tasks"], ["/journal", "Journal"], ["/bookmarks", "Bookmarks"]]) {
  test(`lazy ${heading} supports direct links and refresh`, async ({ page }) => {
    const errors: string[] = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await mockAPI(page);
    await page.goto(path);
    await expect(page.getByRole("heading", { name: heading, exact: true })).toBeVisible();
    await page.reload();
    await expect(page.getByRole("heading", { name: heading, exact: true })).toBeVisible();
    expect(errors).toEqual([]);
  });
}

test("all built text assets have smaller, lossless gzip siblings", async () => {
  const files = ["index.html", "favicon.svg", ...(await readdir(new URL("assets/", build)))
    .filter((name) => /\.(js|css)$/.test(name)).map((name) => `assets/${name}`)];
  for (const name of files) {
    const original = await readFile(new URL(name, build));
    const compressed = await readFile(new URL(`${name}.gz`, build));
    expect(gunzipSync(compressed).equals(original), name).toBe(true);
    expect(compressed.length, name).toBeLessThan(original.length);
  }
});

test("system typography remains readable in light desktop and dark, reduced-motion mobile", async ({ page }) => {
  await mockAPI(page);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.goto("/");
  await expect(page.getByRole("heading", { name: "Overview", exact: true })).toBeVisible();
  await page.screenshot({ path: test.info().outputPath("system-font-desktop.png"), fullPage: true, animations: "disabled" });
  await page.getByRole("button", { name: "Switch to dark mode", exact: true }).click();
  await page.setViewportSize({ width: 375, height: 812 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await expect(page.locator("html")).toHaveClass(/dark/);
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("system-font-mobile-dark.png"), fullPage: true, animations: "disabled" });
  await page.setViewportSize({ width: 812, height: 375 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});
