import { test, expect, type Page } from "@playwright/test";

const resources = [
  { id: 1, name: "notes.md", kind: "document", size: 43 },
  { id: 2, name: "image.png", kind: "image", size: 68 },
  { id: 3, name: "clip.mp4", kind: "video", size: 128 },
  { id: 4, name: "song.mp3", kind: "audio", size: 128 },
  { id: 5, name: "paper.pdf", kind: "document", size: 128 },
  { id: 6, name: "archive.zip", kind: "archive", size: 256 },
  { id: 7, name: "old.txt", kind: "document", size: 0, state: "missing" },
].map((r) => ({
  root_id: 1, path: `/managed/${r.name}`, is_dir: false, state: "present",
  modified_at: "2026-10-14T10:00:00Z", tags: [],
  ...r,
}));

async function setup(page: Page, options: { conflict?: boolean; text?: string; failLoad?: boolean; readOnly?: boolean; delay?: boolean } = {}) {
  let content = options.text ?? "# Project notes\n\nHello, Nexus.\n";
  let revision = "a".repeat(64);
  const writes: { content: string; revision: string }[] = [];
  await page.route("**/api/**", async (route) => {
    const req = route.request();
    const url = new URL(req.url());
    const match = url.pathname.match(/\/api\/files\/resources\/(\d+)(?:\/(preview|content))?$/);
    let response: unknown;
    if (url.pathname === "/api/auth/me") response = { owner: true };
    else if (url.pathname === "/api/files/roots") response = [{ id: 1, path: "/managed" }];
    else if (url.pathname === "/api/files/resources") response = {
      items: resources, total: resources.length, limit: 200, offset: 0,
      kinds: { document: 3, image: 1, video: 1, audio: 1, archive: 1 },
      directories: [],
    };
    else if (match) {
      const id = Number(match[1]);
      const kind = ["text", "image", "video", "audio", "pdf", "unsupported"][id - 1];
      if (match[2] === "preview") {
        if (options.failLoad) return route.fulfill({ status: 404, json: { error: "Underlying file is unavailable" } });
        response = { type: kind, mime: `${kind}/mock`, size: 128, modified_at: "2026-10-14T10:00:00Z", editable: id === 1 && !options.readOnly, reason: kind === "unsupported" ? "This format cannot be previewed. Download it to open locally." : undefined };
      } else if (match[2] === "content" && id === 1) {
        if (req.method() === "PUT") {
          const body = req.postDataJSON();
          writes.push(body);
          if (options.delay) await new Promise((resolve) => setTimeout(resolve, 500));
          if (options.conflict) return route.fulfill({ status: 409, json: { error: "file changed" } });
          content = body.content;
          revision = "b".repeat(64);
        }
        response = { content, revision, encoding: "UTF-8" };
      } else if (match[2] === "content" && id === 2) {
        return route.fulfill({ contentType: "image/png", body: Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+c47sAAAAASUVORK5CYII=", "base64") });
      } else if (match[2] === "content") {
        return route.fulfill({ contentType: kind === "pdf" ? "application/pdf" : `${kind}/mp4`, body: "" });
      } else response = resources.find((r) => r.id === id);
    } else return route.fulfill({ status: 404, json: { error: "not found" } });
    return route.fulfill({ json: response });
  });
  await page.goto("/files");
  await expect(page.getByText("notes.md", { exact: true })).toBeVisible();
  return writes;
}

async function openText(page: Page) {
  await page.getByText("notes.md", { exact: true }).dblclick();
  await expect(page.getByRole("dialog", { name: "notes.md" })).toBeVisible();
  await expect(page.getByTestId("file-text-editor")).toBeVisible();
}

async function edit(page: Page, content: string) {
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  const editor = page.locator(".cm-content[contenteditable=true]");
  await expect(editor).toBeVisible();
  await editor.fill(content);
  await expect(page.getByText("Unsaved", { exact: true })).toBeVisible();
}

test("preview is read-only; keyboard save edits real file content and updates status", async ({ page }) => {
  const writes = await setup(page);
  await openText(page);
  await expect(page.locator(".cm-content")).toHaveAttribute("contenteditable", "false");
  await expect(page.locator(".cm-content")).toHaveAttribute("aria-readonly", "true");
  await expect(page.getByText("Read-only preview", { exact: true })).toBeVisible();
  await edit(page, "# Updated notes\n\n你好，Nexus。\n");
  await page.locator(".cm-content").press("ControlOrMeta+s");
  await expect(page.getByText("Unsaved", { exact: true })).toHaveCount(0);
  expect(writes).toEqual([{ content: "# Updated notes\n\n你好，Nexus。\n", revision: "a".repeat(64) }]);
  await expect(page.getByRole("button", { name: "Save", exact: true })).toBeDisabled();
  // Save does not reset editing history or the cursor.
  await page.locator(".cm-content").press("ControlOrMeta+z");
  await expect(page.getByText("Unsaved", { exact: true })).toBeVisible();
  await expect(page.locator(".cm-content")).toContainText("Project notes");
});

test("closing and next-file navigation confirm unsaved changes without losing rejected draft", async ({ page }) => {
  await setup(page);
  await openText(page);
  await edit(page, "keep this draft");
  page.once("dialog", (dialog) => dialog.dismiss());
  await page.getByRole("button", { name: "Close preview" }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(page.locator(".cm-content")).toContainText("keep this draft");
  page.once("dialog", (dialog) => dialog.dismiss());
  await page.getByRole("button", { name: "Next file" }).click();
  await expect(page.getByRole("dialog", { name: "notes.md" })).toBeVisible();
  page.once("dialog", (dialog) => dialog.accept());
  await page.getByRole("button", { name: "Next file" }).click();
  await expect(page.getByRole("dialog", { name: "image.png" })).toBeVisible();
});

test("discarding edits restores the actual preview document", async ({ page }) => {
  await setup(page);
  await openText(page);
  await edit(page, "discard me");
  page.once("dialog", (dialog) => dialog.accept());
  await page.getByRole("button", { name: "Preview", exact: true }).click();
  await expect(page.locator(".cm-content")).toContainText("Project notes");
  await expect(page.locator(".cm-content")).not.toContainText("discard me");
  await expect(page.locator(".cm-content")).toHaveAttribute("contenteditable", "false");
});

test("a conflicting save keeps draft and offers draft download and reload", async ({ page }) => {
  await setup(page, { conflict: true });
  await openText(page);
  await edit(page, "local draft");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("changed on disk");
  await expect(page.locator(".cm-content")).toContainText("local draft");
  await expect(page.getByRole("button", { name: "Save", exact: true })).toBeDisabled();
  const download = page.waitForEvent("download");
  await page.getByRole("button", { name: "Download draft" }).click();
  expect((await download).suggestedFilename()).toBe("notes.md");
  page.once("dialog", (dialog) => dialog.accept());
  await page.getByRole("button", { name: "Reload", exact: true }).click();
  await expect(page.getByRole("alert")).toHaveCount(0);
  await expect(page.locator(".cm-content")).toContainText("Project notes");
});

test("UTF-8 BOM and CRLF survive editing and saving", async ({ page }) => {
  const writes = await setup(page, { text: "\uFEFFline one\r\nline two\r\n" });
  await openText(page);
  await expect(page.getByText("UTF-8 BOM", { exact: true })).toBeVisible();
  await expect(page.getByText("CRLF", { exact: true })).toBeVisible();
  await edit(page, "updated\nline two\n");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByText("Unsaved", { exact: true })).toHaveCount(0);
  expect(writes[0].content).toBe("\uFEFFupdated\r\nline two\r\n");
});

test("an in-flight save locks editing and navigation; empty content can be saved", async ({ page }) => {
  const writes = await setup(page, { delay: true });
  await openText(page);
  await edit(page, "");
  await page.getByRole("button", { name: "Save", exact: true }).click();
  await expect(page.getByRole("button", { name: "Close preview" })).toBeDisabled();
  await expect(page.getByRole("button", { name: "Next file" })).toBeDisabled();
  await expect(page.locator(".cm-content")).toHaveAttribute("contenteditable", "false");
  await expect(page.getByRole("button", { name: "Close preview" })).toBeEnabled();
  expect(writes[0].content).toBe("");
});

test("read-only text has no edit button", async ({ page }) => {
  await setup(page, { readOnly: true });
  await openText(page);
  await expect(page.getByRole("button", { name: "Edit", exact: true })).toHaveCount(0);
});

test("missing and unsupported files have appropriate fallback", async ({ page }) => {
  await setup(page);
  await page.getByText("old.txt", { exact: true }).click();
  await page.getByRole("button", { name: "Show file details" }).click();
  await expect(page.getByText("This file is missing from disk.")).toBeVisible();
  await expect(page.getByRole("button", { name: "Preview / edit" })).toHaveCount(0);
  await page.getByText("archive.zip", { exact: true }).dblclick();
  await expect(page.getByText("Preview unavailable", { exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "Download file", exact: true })).toHaveAttribute("href", "/api/files/resources/6/download");
});

test("load failure displays a recoverable error", async ({ page }) => {
  await setup(page, { failLoad: true });
  await page.getByText("notes.md", { exact: true }).dblclick();
  await expect(page.getByRole("alert")).toContainText("Underlying file is unavailable");
  await expect(page.getByRole("button", { name: "Reload", exact: true })).toBeEnabled();
});

test("native dialog traps focus and Escape returns it to the entry button", async ({ page }) => {
  await setup(page);
  await page.getByText("notes.md", { exact: true }).click();
  await page.getByRole("button", { name: "Show file details" }).click();
  await page.getByRole("button", { name: "Preview / edit" }).click();
  await expect(page.getByRole("dialog")).toBeVisible();
  await page.getByRole("button", { name: "Next file" }).focus();
  await page.keyboard.press("Tab");
  expect(await page.evaluate(() => !!document.activeElement?.closest("dialog"))).toBe(true);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Preview / edit" })).toBeFocused();
});

test("image zoom, fullscreen and native media viewers", async ({ page }) => {
  await setup(page);
  await page.getByText("image.png", { exact: true }).dblclick();
  const image = page.getByRole("img", { name: "image.png" });
  await expect(image).toBeVisible();
  await expect.poll(async () => (await image.boundingBox())?.width).toBeGreaterThan(1);
  const before = (await image.boundingBox())!.width;
  await page.getByRole("button", { name: "Zoom in" }).click();
  await expect(page.getByRole("button", { name: "125%", exact: true })).toBeVisible();
  await expect.poll(async () => (await image.boundingBox())!.width).toBeCloseTo(before * 1.25, 0);
  await page.getByRole("button", { name: "Fullscreen preview" }).click();
  await expect(page.getByRole("button", { name: "Exit fullscreen" })).toBeVisible();
  await page.getByRole("button", { name: "Next file" }).click();
  await expect(page.locator("video")).toHaveAttribute("controls", "");
  await expect(page.locator("video")).toHaveAttribute("preload", "metadata");
  await page.getByRole("button", { name: "Next file" }).click();
  await expect(page.locator("audio")).toHaveAttribute("controls", "");
  await page.getByRole("button", { name: "Next file" }).click();
  await expect(page.getByTitle("PDF preview: paper.pdf")).toBeVisible();
});

test("small viewport, dark theme and reduced motion retain usable editor controls", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await page.emulateMedia({ reducedMotion: "reduce", colorScheme: "dark" });
  await page.addInitScript(() => localStorage.setItem("nexus-theme", "dark"));
  await setup(page);
  await openText(page);
  await edit(page, "mobile draft");
  const dialog = page.getByRole("dialog");
  const box = await dialog.boundingBox();
  expect(box!.width).toBeLessThanOrEqual(375);
  for (const name of ["Close preview", "Save", "Next file"]) await expect(page.getByRole("button", { name, exact: true })).toBeInViewport();
  await page.getByTitle("Toggle line wrapping").click();
  await expect(page.getByTitle("Toggle line wrapping")).toHaveAttribute("aria-pressed", "true");
});

test("visual QA: desktop, dark mobile and landscape preview", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await setup(page, { text: [
    "# Nexus · Project notes", "",
    "A personal workbench for the things that matter.", "",
    "## Today's checklist", "",
    "- [x] Preview files without leaving the workbench",
    "- [x] Edit text and configuration safely",
    "- [ ] Write down what I learned", "",
    "## Configuration", "", "```json",
    '{ "preview": true, "editor": "CodeMirror", "theme": "system" }',
    "```", "", "支持中文内容，保留原始文件的编码与换行。", "",
  ].join("\n") });
  await openText(page);
  await page.getByRole("button", { name: "Edit", exact: true }).click();
  await expect(page.locator(".cm-content")).toContainText("Configuration");
  await expect(page.locator(".cm-line span").first()).toBeVisible();
  await page.screenshot({ path: test.info().outputPath("preview-desktop.png") });
  await page.setViewportSize({ width: 375, height: 812 });
  await page.evaluate(() => { document.documentElement.classList.add("dark"); document.documentElement.style.colorScheme = "dark"; });
  await page.getByTitle("Toggle line wrapping").click();
  await expect(page.getByRole("button", { name: "Save", exact: true })).toBeInViewport();
  await page.screenshot({ path: test.info().outputPath("preview-mobile-dark.png") });
  await page.setViewportSize({ width: 812, height: 375 });
  await expect(page.getByRole("button", { name: "Close preview" })).toBeInViewport();
  await expect(page.getByRole("button", { name: "Save", exact: true })).toBeInViewport();
  await page.screenshot({ path: test.info().outputPath("preview-landscape.png") });
});
