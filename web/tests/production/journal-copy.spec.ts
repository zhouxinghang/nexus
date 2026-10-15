import { test, expect, type Page } from "@playwright/test";

const loopback = "http://127.0.0.1:4174";
const ordinaryHTTP = "http://nexus-http.test:4174";
const text = "第一行 · Journal\n  second line with spaces\n<script>literal text, not markup</script>\n";

declare global {
  interface Window {
    journalCopyProbe: { modern: string[]; legacy: number };
  }
}

// Real clipboard writes are asserted below, so keep this file serial to avoid
// different tests overwriting each other's clipboard fixtures.
test.describe.configure({ mode: "serial" });

async function setup(page: Page, options: { insecure?: boolean; rejectModern?: boolean; legacy?: "false" | "throw" } = {}) {
  await page.addInitScript((settings) => {
    window.journalCopyProbe = { modern: [], legacy: 0 };
    const nativeCopy = document.execCommand.bind(document);
    document.execCommand = (command, showUI, value) => {
      if (command === "copy") {
        window.journalCopyProbe.legacy++;
        if (settings.legacy === "false") return false;
        if (settings.legacy === "throw") throw new DOMException("Clipboard blocked", "NotAllowedError");
      }
      return nativeCopy(command, showUI, value);
    };
    if (navigator.clipboard?.writeText) {
      const nativeWrite = navigator.clipboard.writeText.bind(navigator.clipboard);
      navigator.clipboard.writeText = async (value) => {
        window.journalCopyProbe.modern.push(value);
        if (settings.rejectModern) throw new DOMException("Write permission denied", "NotAllowedError");
        return nativeWrite(value);
      };
    }
  }, options);
  if (options.insecure) {
    // Serve the real production bundle at a genuine non-secure HTTP origin.
    // Proxy only network transport; do not fake isSecureContext or clipboard.
    await page.route(`${ordinaryHTTP}/**`, async (route) => {
      const url = new URL(route.request().url());
      const response = await route.fetch({ url: loopback + url.pathname + url.search });
      await route.fulfill({ response });
    });
  }
  await page.route("**/api/**", (route) => {
    const path = new URL(route.request().url()).pathname;
    if (path === "/api/auth/me") return route.fulfill({ json: { authenticated: true } });
    if (path === "/api/journal/entries") return route.fulfill({ json: {
      items: [{ id: 1, body: text, created_at: "2026-10-15T00:00:00Z" }],
      next_cursor: null,
    } });
    return route.fulfill({ status: 404, json: { error: "not found" } });
  });
  await page.goto(`${options.insecure ? ordinaryHTTP : loopback}/journal`);
  await expect(page.getByText(text.trim(), { exact: true })).toBeVisible();
}

async function clickCopy(page: Page) {
  const button = page.getByRole("button", { name: "Copy entry", exact: true });
  await button.focus();
  await button.click();
}

async function readCopiedText(page: Page) {
  // Read from a secure origin after the click. This grants the verifier access,
  // not the HTTP source, and never replaces either clipboard implementation.
  const context = page.context();
  await context.grantPermissions(["clipboard-read", "clipboard-write"], { origin: loopback });
  const verifier = await context.newPage();
  try {
    await verifier.route("**/api/**", (route) => route.fulfill({ status: 401, json: { error: "unauthorized" } }));
    await verifier.goto(`${loopback}/login`);
    return await verifier.evaluate(() => navigator.clipboard.readText());
  } finally {
    await verifier.close();
  }
}

test("ordinary HTTP copies Journal Entry using the native legacy clipboard path", async ({ page }) => {
  await setup(page, { insecure: true });
  expect(await page.evaluate(() => window.isSecureContext)).toBe(false);
  expect(await page.evaluate(() => typeof navigator.clipboard)).toBe("undefined");
  await clickCopy(page);
  await expect(page.getByText("Copied to clipboard", { exact: true })).toBeVisible({ timeout: 1000 });
  await expect(page.getByRole("button", { name: "Copied", exact: true })).toBeFocused();
  expect(await page.evaluate(() => window.journalCopyProbe.legacy)).toBe(1);
  expect(await readCopiedText(page)).toBe(text);
  await expect(page.getByText("Clipboard unavailable", { exact: true })).toHaveCount(0);
  expect(await page.locator("textarea").count()).toBe(1);
});

test("permission rejection falls back without changing copied text or focus", async ({ page }) => {
  await setup(page, { rejectModern: true });
  await clickCopy(page);
  await expect(page.getByText("Copied to clipboard", { exact: true })).toBeVisible({ timeout: 1000 });
  expect(await page.evaluate(() => window.journalCopyProbe)).toEqual({ modern: [text], legacy: 1 });
  await expect(page.getByRole("button", { name: "Copied", exact: true })).toBeFocused();
  expect(await readCopiedText(page)).toBe(text);
});

test("a permitted modern clipboard stays the first choice and resets the copied indicator", async ({ page, context }) => {
  await context.grantPermissions(["clipboard-read", "clipboard-write"], { origin: loopback });
  await page.clock.install();
  await setup(page);
  await clickCopy(page);
  await expect(page.getByRole("button", { name: "Copied", exact: true })).toBeFocused();
  expect(await page.evaluate(() => window.journalCopyProbe)).toEqual({ modern: [text], legacy: 0 });
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(text);
  await page.clock.fastForward(1500);
  await expect(page.getByRole("button", { name: "Copy entry", exact: true })).toBeFocused();
});

for (const failure of ["false", "throw"] as const) {
  test(`both clipboard paths blocked (${failure}) offer selected, immutable manual-copy text`, async ({ page }) => {
    await setup(page, { rejectModern: true, legacy: failure });
    await clickCopy(page);
    const dialog = page.getByRole("dialog", { name: "Copy Journal Entry manually" });
    await expect(dialog).toBeVisible({ timeout: 1000 });
    const field = dialog.getByRole("textbox", { name: "Journal Entry text" });
    await expect(field).toHaveValue(text);
    await expect(field).toHaveAttribute("readonly", "");
    await expect(field).toBeFocused();
    expect(await field.evaluate((element: HTMLTextAreaElement) => [element.selectionStart, element.selectionEnd])).toEqual([0, text.length]);
    await expect(dialog).toContainText("Ctrl/Cmd+C");
    await expect(page.getByText("Copied to clipboard", { exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Copied", exact: true })).toHaveCount(0);
    expect(await page.locator("textarea").count()).toBe(2);
    await page.keyboard.press("Tab");
    expect(await page.evaluate(() => !!document.activeElement?.closest("dialog"))).toBe(true);
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Copy entry", exact: true })).toBeFocused();
    expect(await page.locator("textarea").count()).toBe(1);
  });
}

test("ordinary HTTP with no clipboard path still offers manual copying", async ({ page }) => {
  await setup(page, { insecure: true, legacy: "false" });
  await clickCopy(page);
  await expect(page.getByRole("dialog", { name: "Copy Journal Entry manually" })).toBeVisible({ timeout: 1000 });
  expect(await page.evaluate(() => window.journalCopyProbe)).toEqual({ modern: [], legacy: 1 });
});

test("manual copying can select again and close without modifying the append draft", async ({ page }) => {
  await setup(page, { rejectModern: true, legacy: "false" });
  const draft = page.getByPlaceholder("What happened?");
  await draft.fill("Unsubmitted draft must survive copying");
  await clickCopy(page);
  const dialog = page.getByRole("dialog", { name: "Copy Journal Entry manually" });
  const field = dialog.getByRole("textbox", { name: "Journal Entry text" });
  await expect(dialog).toBeVisible({ timeout: 1000 });
  await field.evaluate((element: HTMLTextAreaElement) => element.setSelectionRange(2, 3));
  await dialog.getByRole("button", { name: "Select all text", exact: true }).click();
  await expect(field).toBeFocused();
  expect(await field.evaluate((element: HTMLTextAreaElement) => [element.selectionStart, element.selectionEnd])).toEqual([0, text.length]);
  await dialog.getByRole("button", { name: "Close manual copy", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(draft).toHaveValue("Unsubmitted draft must survive copying");
  await expect(page.getByRole("button", { name: "Copy entry", exact: true })).toBeFocused();
});

test("manual copy remains accessible on light desktop and dark, reduced-motion mobile", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await setup(page, { rejectModern: true, legacy: "false" });
  await clickCopy(page);
  const dialog = page.getByRole("dialog", { name: "Copy Journal Entry manually" });
  await expect(dialog).toBeVisible({ timeout: 1000 });
  await page.screenshot({ path: test.info().outputPath("journal-copy-desktop.png"), animations: "disabled" });
  await page.setViewportSize({ width: 375, height: 812 });
  await page.emulateMedia({ reducedMotion: "reduce" });
  await page.evaluate(() => { document.documentElement.classList.add("dark"); document.documentElement.style.colorScheme = "dark"; });
  await expect(dialog.getByRole("button", { name: "Select all text", exact: true })).toBeInViewport();
  await expect(dialog.getByRole("button", { name: "Close manual copy", exact: true })).toBeInViewport();
  expect((await dialog.boundingBox())!.width).toBeLessThanOrEqual(375);
  await page.screenshot({ path: test.info().outputPath("journal-copy-mobile-dark.png"), animations: "disabled" });
  await page.setViewportSize({ width: 812, height: 375 });
  await expect(dialog.getByRole("button", { name: "Select all text", exact: true })).toBeInViewport();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
});
