import { test, expect, type Page } from "@playwright/test";

async function setup(page: Page, options: { many?: boolean; mixed?: boolean; slowSort?: boolean; fail?: boolean } = {}) {
  const requests: URL[] = [];
  const dirs = [
    { path: "/managed/projects", name: "projects", modified_at: "2026-10-14T12:00:00Z", file_created_at: "2026-10-11T12:00:00Z", state: "present" },
    { path: "/managed/projects/design", name: "design", modified_at: "2026-10-13T12:00:00Z", file_created_at: "2026-10-11T12:00:00Z", state: "present" },
    { path: "/managed/empty", name: "empty", modified_at: "2026-10-12T12:00:00Z", file_created_at: "2026-10-12T12:00:00Z", state: "present" },
    { path: "/managed/old", name: "old", modified_at: null, file_created_at: null, state: "missing" },
  ];
  const files = [
    { id: 1, root_id: 1, path: "/managed/a.txt", size: 300, modified_at: "2026-10-12T12:00:00Z", file_created_at: "2026-10-13T12:00:00Z" },
    { id: 2, root_id: 1, path: "/managed/b.txt", size: 100, modified_at: "2026-10-14T12:00:00Z", file_created_at: "2026-10-11T12:00:00Z" },
    { id: 3, root_id: 1, path: "/managed/c.txt", size: 200, modified_at: "2026-10-13T12:00:00Z", file_created_at: null },
    { id: 4, root_id: 1, path: "/managed/projects/readme.txt", size: 50, modified_at: "2026-10-14T12:00:00Z" },
    { id: 5, root_id: 1, path: "/managed/projects/design/mockup.txt", size: 40, modified_at: "2026-10-14T12:00:00Z" },
    { id: 6, root_id: 1, path: "/managed/old/missing.txt", size: 10, modified_at: "2026-10-10T12:00:00Z", state: "missing", tags: ["keep"] },
    { id: 7, root_id: 2, path: "/archive/archived.txt", size: 10, modified_at: "2026-10-10T12:00:00Z" },
    ...(options.mixed ? [
      { id: 8, root_id: 1, path: "/managed/media/a-photo.png", size: 4096, modified_at: "2026-10-14T12:00:00Z", kind: "image" },
      { id: 9, root_id: 1, path: "/managed/media/b-song.mp3", size: 8192, modified_at: "2026-10-14T12:00:00Z", kind: "audio" },
    ] : []),
    ...(options.many ? Array.from({ length: 1205 }, (_, i) => ({
      id: 100 + i, root_id: 1, path: `/managed/file-${String(i).padStart(3, "0")}.txt`,
      size: 1000 + i, modified_at: "2026-10-14T12:00:00Z",
    })) : []),
  ].map((f) => ({ name: f.path.split("/").at(-1)!, is_dir: false, kind: "document", state: "present", file_created_at: null as string | null, tags: [] as string[], ...f }));
  let failed = false;
  await page.route("**/api/**", async (route) => {
    const req = route.request();
    const url = new URL(req.url());
    if (url.pathname === "/api/auth/me") return route.fulfill({ json: { authenticated: true } });
    if (url.pathname === "/api/files/roots") return route.fulfill({ json: [{ id: 1, path: "/managed" }, { id: 2, path: "/archive" }] });
    if (url.pathname === "/api/files/resources") {
      requests.push(url);
      const parent = url.searchParams.get("parent");
      const folderMode = url.searchParams.get("mode") === "folder";
      const rootID = Number(url.searchParams.get("root_id"));
      const sort = url.searchParams.get("sort") ?? "name";
      const order = url.searchParams.get("order") === "desc" ? -1 : 1;
      if (options.fail && !failed) {
        failed = true;
        return route.fulfill({ status: 403, json: { error: "directory is not readable" } });
      }
      if (options.slowSort && sort === "size" && order === -1) await new Promise((resolve) => setTimeout(resolve, 600));
      let items = files.filter((f) => (!rootID || f.root_id === rootID) && (!parent || f.path.slice(0, f.path.lastIndexOf("/")) === parent));
      const dir = url.searchParams.get("dir");
      if (dir) items = items.filter((f) => f.path.startsWith(dir.replace(/\/$/, "") + "/"));
      const name = url.searchParams.get("name");
      if (name) items = items.filter((f) => f.name.toLowerCase().includes(name.toLowerCase()));
      const tag = url.searchParams.get("tag");
      if (tag) items = items.filter((f) => f.tags.includes(tag));
      const counts: Record<string, number> = {};
      for (const item of items) counts[item.kind] = (counts[item.kind] || 0) + 1;
      const kind = url.searchParams.get("kind");
      if (kind) items = items.filter((item) => item.kind === kind);
      items.sort((a, b) => {
        if (sort === "created" && (!a.file_created_at || !b.file_created_at)) {
          return a.file_created_at ? -1 : b.file_created_at ? 1 : a.path.localeCompare(b.path);
        }
        const diff = sort === "size" ? a.size - b.size
          : sort === "modified" ? Date.parse(a.modified_at) - Date.parse(b.modified_at)
          : sort === "created" ? Date.parse(a.file_created_at!) - Date.parse(b.file_created_at!)
          : a.name.localeCompare(b.name);
        return diff * order || a.path.localeCompare(b.path);
      });
      const offset = Number(url.searchParams.get("offset") ?? 0);
      const limit = folderMode ? 0 : Number(url.searchParams.get("limit") ?? 200);
      const directDirs = dirs.filter((d) => d.path.slice(0, d.path.lastIndexOf("/")) === parent);
      return route.fulfill({ json: {
        items: folderMode ? items : items.slice(offset, offset + limit), total: items.length, limit, offset,
        kinds: counts, directories: directDirs,
      } });
    }
    const match = url.pathname.match(/^\/api\/files\/resources\/(\d+)\/tags(?:\/(.+))?$/);
    if (match) {
      const file = files.find((f) => f.id === Number(match[1]))!;
      if (req.method() === "POST") file.tags.push(req.postDataJSON().tag);
      if (req.method() === "DELETE") file.tags = file.tags.filter((tag) => tag !== decodeURIComponent(match[2]));
      return route.fulfill({ json: file });
    }
    return route.fulfill({ status: 404, json: { error: "not found" } });
  });
  await page.goto("/files");
  await expect(page.getByRole("navigation", { name: "Directory breadcrumbs" })).toBeVisible();
  if (!options.fail) await expect(page.getByRole("button", { name: "Open folder projects" })).toBeVisible();
  return requests;
}

function fileNames(page: Page) {
  return page.locator('tr[data-resource-id] td:nth-child(2) span[title].font-medium');
}

test("directory hierarchy, breadcrumbs and browser back/forward never flatten descendants", async ({ page }) => {
  const requests = await setup(page);
  await expect(fileNames(page)).toHaveText(["a.txt", "b.txt", "c.txt"]);
  await expect(page.getByText("readme.txt", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "Open folder projects" }).dblclick();
  await expect(fileNames(page)).toHaveText(["readme.txt"]);
  await page.getByRole("button", { name: "Open folder design" }).focus();
  await page.keyboard.press("Enter");
  await expect(fileNames(page)).toHaveText(["mockup.txt"]);
  const crumbs = page.getByRole("navigation", { name: "Directory breadcrumbs" });
  await expect(crumbs.getByRole("button", { name: "design", exact: true })).toHaveAttribute("aria-current", "location");
  await page.goBack();
  await expect(fileNames(page)).toHaveText(["readme.txt"]);
  await page.goForward();
  await expect(fileNames(page)).toHaveText(["mockup.txt"]);
  await crumbs.getByRole("button", { name: "/managed", exact: true }).click();
  await expect(fileNames(page)).toHaveText(["a.txt", "b.txt", "c.txt"]);
  expect(requests.every((url) => url.searchParams.has("parent") && url.searchParams.has("root_id"))).toBe(true);
});

test("empty directories are browsable and roots remain separate", async ({ page }) => {
  await setup(page);
  await page.getByRole("button", { name: "Open folder empty" }).dblclick();
  await expect(page.getByText("Empty directory", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Browse /archive", exact: true }).click();
  await expect(fileNames(page)).toHaveText(["archived.txt"]);
  await page.getByRole("navigation", { name: "Directory breadcrumbs" }).getByRole("button", { name: "All roots", exact: true }).click();
  await expect(page.getByRole("button", { name: "/managed", exact: true })).toBeVisible();
  await expect(page.locator("tbody tr")).toHaveCount(0);
  await page.getByRole("button", { name: "Browse /managed", exact: true }).click();
  await expect(fileNames(page)).toHaveText(["a.txt", "b.txt", "c.txt"]);
});

test("notes are removed while tags remain keyboard-editable, including Missing Resources", async ({ page }) => {
  await setup(page);
  await page.getByText("a.txt", { exact: true }).click();
  await expect(page.getByRole("region", { name: "File details" })).toHaveCount(0);
  await page.getByRole("button", { name: "Show file details" }).click();
  await expect(page.getByText("Note", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Save note", exact: true })).toHaveCount(0);
  await page.getByRole("textbox", { name: "Tags", exact: true }).fill("important");
  await page.getByRole("textbox", { name: "Tags", exact: true }).press("Enter");
  await expect(page.getByRole("button", { name: "Remove tag important" })).toBeVisible();
  await page.getByRole("button", { name: "Remove tag important" }).focus();
  await page.keyboard.press("Enter");
  await expect(page.getByRole("button", { name: "Remove tag important" })).toHaveCount(0);
  await page.getByRole("button", { name: "Open folder old" }).dblclick();
  await page.getByText("missing.txt", { exact: true }).click();
  await page.getByRole("button", { name: "Show file details" }).click();
  await expect(page.getByText("This file is missing from disk.")).toBeVisible();
  await expect(page.getByRole("button", { name: "Remove tag keep" })).toBeVisible();
});

test("Size and Modified headers sort in both directions with correct aria-sort", async ({ page }) => {
  await setup(page);
  const size = page.getByRole("columnheader", { name: "Size", exact: true });
  const modified = page.getByRole("columnheader", { name: "Modified", exact: true });
  await size.getByRole("button").click();
  await expect(size).toHaveAttribute("aria-sort", "descending");
  await expect(fileNames(page)).toHaveText(["a.txt", "c.txt", "b.txt"]);
  await size.getByRole("button").click();
  await expect(size).toHaveAttribute("aria-sort", "ascending");
  await expect(fileNames(page)).toHaveText(["b.txt", "c.txt", "a.txt"]);
  await modified.getByRole("button").click();
  await expect(modified).toHaveAttribute("aria-sort", "descending");
  await expect(fileNames(page)).toHaveText(["b.txt", "c.txt", "a.txt"]);
  await modified.getByRole("button").click();
  await expect(modified).toHaveAttribute("aria-sort", "ascending");
  await expect(fileNames(page)).toHaveText(["a.txt", "c.txt", "b.txt"]);
  await page.reload();
  await expect(fileNames(page)).toHaveText(["a.txt", "c.txt", "b.txt"]);
  await expect(modified).toHaveAttribute("aria-sort", "ascending");
});

test("Created sorts actual file creation times, keeps unknowns last, and restores URL state", async ({ page }) => {
  const requests = await setup(page);
  const created = page.getByRole("columnheader", { name: "Created", exact: true });
  await created.getByRole("button").focus();
  await page.keyboard.press("Enter");
  await expect(created).toHaveAttribute("aria-sort", "descending");
  await expect(fileNames(page)).toHaveText(["a.txt", "b.txt", "c.txt"]);
  expect(requests.at(-1)!.searchParams.get("sort")).toBe("created");
  await created.getByRole("button").click();
  await expect(created).toHaveAttribute("aria-sort", "ascending");
  await expect(fileNames(page)).toHaveText(["b.txt", "a.txt", "c.txt"]);
  await expect(page).toHaveURL(/sort=created/);
  await page.reload();
  await expect(created).toHaveAttribute("aria-sort", "ascending");
  await expect(fileNames(page)).toHaveText(["b.txt", "a.txt", "c.txt"]);
  await page.screenshot({ path: test.info().outputPath("files-created-desktop.png"), animations: "disabled" });
  await page.getByRole("button", { name: "By type", exact: true }).click();
  await expect(fileNames(page).first()).toHaveText("b.txt");
  await expect(created).toHaveAttribute("aria-sort", "ascending");
});

test("mobile Created sorting is accessible without horizontal overflow", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await page.emulateMedia({ reducedMotion: "reduce", colorScheme: "dark" });
  await setup(page);
  await page.evaluate(() => document.documentElement.classList.add("dark"));
  await page.getByRole("combobox", { name: "Sort by" }).selectOption("created");
  await expect(fileNames(page)).toHaveText(["a.txt", "b.txt", "c.txt"]);
  await page.getByRole("button", { name: "Sort ascending" }).click();
  await expect(fileNames(page)).toHaveText(["b.txt", "a.txt", "c.txt"]);
  await page.getByText("a.txt", { exact: true }).click();
  await page.getByRole("button", { name: "Show file details" }).click();
  await expect(page.getByRole("region", { name: "File details" }).getByText("Created on Host")).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("files-created-mobile-dark.png") });
});

test("folder mode loads every child in one unpaged request and clears batch selection on navigation", async ({ page }) => {
  const requests = await setup(page, { many: true });
  await expect(fileNames(page)).toHaveCount(1208);
  await page.getByRole("columnheader", { name: "Size", exact: true }).getByRole("button").click();
  await expect(fileNames(page).first()).toHaveText("file-1204.txt");
  await expect(fileNames(page)).toHaveCount(1208);
  await expect(page.getByRole("button", { name: "Load more", exact: true })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Next", exact: true })).toHaveCount(0);
  expect(requests.every((url) => url.searchParams.get("mode") === "folder" && !url.searchParams.has("limit") && !url.searchParams.has("offset"))).toBe(true);
  await page.getByRole("checkbox", { name: "Select a.txt", exact: true }).check();
  await expect(page.getByRole("button", { name: "Clear selection", exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Open folder projects" }).dblclick();
  await expect(fileNames(page)).toHaveText(["readme.txt"]);
  await expect(page.getByRole("button", { name: "Clear selection", exact: true })).toHaveCount(0);
});

test("type mode is recursive, combines URL-backed filters, and works across all roots", async ({ page }) => {
  const requests = await setup(page);
  await page.getByRole("button", { name: "By type", exact: true }).click();
  await expect(fileNames(page)).toHaveCount(6);
  await expect(page.getByRole("rowheader", { name: /Documents.*on this page/ })).toBeVisible();
  await expect(page.getByRole("navigation", { name: "Directory breadcrumbs" })).toHaveCount(0);
  await page.getByRole("navigation", { name: "Managed Roots" }).getByRole("button", { name: "All roots", exact: true }).click();
  await expect(fileNames(page)).toHaveCount(7);
  await expect(page.getByText("archived.txt", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Documents", exact: false }).click();
  await page.getByRole("searchbox", { name: "Filter by name" }).fill("missing");
  await page.getByRole("button", { name: "Filters", exact: true }).click();
  await page.getByRole("textbox", { name: "Folder", exact: true }).fill("/managed/old");
  await page.getByRole("textbox", { name: "Tag", exact: true }).fill("keep");
  await page.getByRole("button", { name: "Apply", exact: true }).click();
  await expect(fileNames(page)).toHaveText(["missing.txt"]);
  await expect(page).toHaveURL(/folder=%2Fmanaged%2Fold/);
  const last = requests.at(-1)!;
  expect(last.searchParams.get("dir")).toBe("/managed/old");
  expect(last.searchParams.get("tag")).toBe("keep");
  expect(last.searchParams.get("name")).toBe("missing");
  expect(last.searchParams.get("kind")).toBe("document");
  expect(last.searchParams.has("parent")).toBe(false);
  await page.reload();
  await expect(fileNames(page)).toHaveText(["missing.txt"]);
  await page.getByRole("button", { name: "Folders", exact: true }).click();
  await expect(fileNames(page)).toHaveText(["a.txt", "b.txt", "c.txt"]);
  await expect(page.getByRole("button", { name: "Filters", exact: true })).toHaveCount(0);
});

test("type pages load one server page at a time and restore filters/page via browser history", async ({ page }) => {
  const requests = await setup(page, { many: true });
  await page.getByRole("button", { name: "By type", exact: true }).click();
  await expect(fileNames(page)).toHaveCount(200);
  await page.getByRole("columnheader", { name: "Size", exact: true }).getByRole("button").click();
  await expect(fileNames(page).first()).toHaveText("file-1204.txt");
  await page.getByRole("checkbox", { name: "Select file-1204.txt", exact: true }).check();
  await page.getByRole("button", { name: "Next", exact: true }).click();
  await expect(fileNames(page)).toHaveCount(200);
  await expect(fileNames(page).first()).toHaveText("file-1004.txt");
  await expect(page.getByRole("button", { name: "Clear selection", exact: true })).toHaveCount(0);
  expect(requests.at(-1)!.searchParams.get("offset")).toBe("200");
  await page.goBack();
  await expect(fileNames(page).first()).toHaveText("file-1204.txt");
  await page.goForward();
  await expect(fileNames(page).first()).toHaveText("file-1004.txt");
  await page.getByRole("searchbox", { name: "Filter by name" }).fill("readme");
  await expect(fileNames(page)).toHaveText(["readme.txt"]);
  expect(requests.at(-1)!.searchParams.get("offset")).toBe("0");
  await expect(page.getByRole("button", { name: "Next", exact: true })).toBeDisabled();
});

test("type groups and range selection follow the displayed order", async ({ page }) => {
  await setup(page, { mixed: true });
  await page.getByRole("button", { name: "By type", exact: true }).click();
  await expect(fileNames(page)).toHaveText(["a.txt", "b.txt", "c.txt", "missing.txt", "mockup.txt", "readme.txt", "a-photo.png", "b-song.mp3"]);
  await expect(page.getByRole("rowheader", { name: "Images1 on this page", exact: true })).toBeVisible();
  await expect(page.getByRole("rowheader", { name: "Audio1 on this page", exact: true })).toBeVisible();
  await page.getByText("readme.txt", { exact: true }).click();
  await page.getByText("a-photo.png", { exact: true }).click({ modifiers: ["Shift"] });
  await expect(page.getByText("2 selected", { exact: true })).toBeVisible();
  await expect(page.getByRole("checkbox", { name: "Select readme.txt", exact: true })).toBeChecked();
  await expect(page.getByRole("checkbox", { name: "Select a-photo.png", exact: true })).toBeChecked();
  await page.getByRole("navigation", { name: "Resource types" }).getByRole("button", { name: "Images 1", exact: true }).click();
  await expect(fileNames(page)).toHaveText(["a-photo.png"]);
  await expect(page.getByRole("button", { name: "Clear selection", exact: true })).toHaveCount(0);
});

test("Finder-style single selection, double-click opening, range selection and parent shortcut", async ({ page }) => {
  await setup(page);
  await page.getByRole("button", { name: "Open folder projects" }).click();
  await expect(fileNames(page)).toHaveText(["a.txt", "b.txt", "c.txt"]);
  await page.getByRole("button", { name: "Open folder projects" }).dblclick();
  await expect(fileNames(page)).toHaveText(["readme.txt"]);
  await page.locator("tr[data-resource-id]").focus();
  await page.keyboard.press("ControlOrMeta+ArrowUp");
  await expect(fileNames(page)).toHaveText(["a.txt", "b.txt", "c.txt"]);
  await page.getByText("a.txt", { exact: true }).click();
  await page.getByText("c.txt", { exact: true }).click({ modifiers: ["Shift"] });
  await expect(page.getByText("3 selected", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Clear selection" }).click();
  await page.locator('tr[data-resource-id="1"]').focus();
  await page.keyboard.press("ArrowDown");
  await expect(page.locator('tr[data-resource-id="2"]')).toBeFocused();
});

test("root management is disclosed in a dismissible native dialog", async ({ page }) => {
  await setup(page);
  await expect(page.getByRole("textbox", { name: "Managed Root path" })).toHaveCount(0);
  const trigger = page.getByRole("button", { name: "Manage roots" });
  await trigger.click();
  await expect(page.getByRole("dialog", { name: "Managed Roots" })).toBeVisible();
  await expect(page.getByRole("textbox", { name: "Managed Root path" })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(trigger).toBeFocused();
});

test("late sort responses cannot overwrite the current directory or sort", async ({ page }) => {
  await page.setViewportSize({ width: 375, height: 812 });
  await setup(page, { slowSort: true });
  const pending = page.waitForRequest((req) => new URL(req.url()).searchParams.get("sort") === "size");
  await page.getByRole("combobox", { name: "Sort by" }).selectOption("size");
  await pending;
  await page.getByRole("combobox", { name: "Sort by" }).selectOption("modified");
  await expect(fileNames(page)).toHaveText(["b.txt", "c.txt", "a.txt"]);
  await page.waitForTimeout(700);
  await expect(fileNames(page)).toHaveText(["b.txt", "c.txt", "a.txt"]);
});

test("directory loading errors are recoverable", async ({ page }) => {
  await setup(page, { fail: true });
  await expect(page.getByRole("alert")).toContainText("directory is not readable");
  await page.getByRole("button", { name: "Retry", exact: true }).click();
  await expect(fileNames(page)).toHaveText(["a.txt", "b.txt", "c.txt"]);
});

test("visual QA: desktop and dark mobile retain directory navigation and sorting", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await setup(page);
  await page.screenshot({ path: test.info().outputPath("files-desktop.png") });
  await page.setViewportSize({ width: 375, height: 812 });
  await page.emulateMedia({ reducedMotion: "reduce", colorScheme: "dark" });
  await page.evaluate(() => document.documentElement.classList.add("dark"));
  await expect(page.getByRole("combobox", { name: "Sort by" })).toBeVisible();
  await page.getByRole("combobox", { name: "Sort by" }).selectOption("modified");
  await page.getByRole("button", { name: "Sort ascending" }).click();
  await expect(fileNames(page)).toHaveText(["a.txt", "c.txt", "b.txt"]);
  await page.getByRole("button", { name: "Open folder projects" }).dblclick();
  await expect(fileNames(page)).toHaveText(["readme.txt"]);
  await page.screenshot({ path: test.info().outputPath("files-mobile-dark.png") });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

test("visual QA: type view, filters and root management work on desktop and mobile", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1000 });
  await setup(page, { mixed: true });
  await page.getByRole("button", { name: "By type", exact: true }).click();
  await expect(fileNames(page)).toHaveCount(8);
  await page.screenshot({ path: test.info().outputPath("files-type-desktop.png") });
  await page.setViewportSize({ width: 375, height: 812 });
  await page.emulateMedia({ reducedMotion: "reduce", colorScheme: "dark" });
  await page.evaluate(() => document.documentElement.classList.add("dark"));
  await page.getByRole("button", { name: "Filters", exact: true }).click();
  await expect(page.getByRole("textbox", { name: "Folder", exact: true })).toBeVisible();
  await page.getByRole("combobox", { name: "Resource type" }).selectOption("image");
  await expect(fileNames(page)).toHaveText(["a-photo.png"]);
  await page.screenshot({ path: test.info().outputPath("files-type-mobile-dark.png") });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.getByRole("button", { name: "Manage roots" }).click();
  await expect(page.getByRole("dialog", { name: "Managed Roots" })).toBeVisible();
  await page.screenshot({ path: test.info().outputPath("files-roots-mobile-dark.png") });
  await page.keyboard.press("Escape");
  await page.setViewportSize({ width: 812, height: 375 });
  await expect(page.getByRole("searchbox", { name: "Filter by name" })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});
