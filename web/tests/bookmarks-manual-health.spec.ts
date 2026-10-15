import { test, expect, type Page } from "@playwright/test";

function bookmark(id: number, name: string, url: string) {
  return {
    id, name, url, icon: "🔖", health: "unknown",
    last_status: null as number | null,
    last_latency_ms: null as number | null,
    last_checked_at: null as string | null,
    fetched_title: null, fetched_favicon_url: null,
    click_count: 0, last_clicked_at: null,
  };
}

async function setup(page: Page) {
  const bookmarks = [
    bookmark(1, "Example", "https://example.com"),
    bookmark(2, "Another", "https://another.example.com"),
  ];
  const checks: string[] = [];
  let listRequests = 0;

  function markUp(b: (typeof bookmarks)[number]) {
    b.health = "up";
    b.last_status = 200;
    b.last_latency_ms = 5;
    b.last_checked_at = new Date().toISOString();
  }

  await page.route("**/api/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path === "/api/auth/me") return route.fulfill({ json: { authenticated: true } });
    if (path === "/api/bookmarks" && request.method() === "GET") {
      listRequests++;
      return route.fulfill({ json: bookmarks });
    }
    if (path === "/api/bookmarks" && request.method() === "POST") {
      const body = request.postDataJSON();
      const created = bookmark(bookmarks.length + 1, body.name, body.url);
      bookmarks.push(created);
      return route.fulfill({ status: 201, json: created });
    }
    if (path === "/api/bookmarks/check") {
      checks.push(path);
      bookmarks.forEach(markUp);
      return route.fulfill({ json: bookmarks });
    }
    const singleCheck = path.match(/^\/api\/bookmarks\/(\d+)\/check$/);
    if (singleCheck) {
      checks.push(path);
      const b = bookmarks.find((b) => b.id === Number(singleCheck[1]))!;
      markUp(b);
      return route.fulfill({ json: b });
    }
    return route.fulfill({ status: 404, json: { error: "not found" } });
  });

  await page.goto("/bookmarks");
  await expect(page.getByText("Unchecked", { exact: true })).toHaveCount(2);
  return { checks, listRequests: () => listRequests };
}

test("loading, polling, and adding Bookmarks never trigger health checks", async ({ page }) => {
  await page.clock.install();
  const { checks, listRequests } = await setup(page);
  await expect(page.getByText(/health checks run only when you choose/)).toBeVisible();
  expect(checks).toEqual([]);

  // Past the former five-minute probe interval, polling remains read-only.
  await page.clock.fastForward(6 * 60_000);
  await expect.poll(listRequests).toBeGreaterThan(1);
  expect(checks).toEqual([]);
  await expect(page.getByText("Unchecked", { exact: true })).toHaveCount(2);

  await page.getByPlaceholder("Name (optional — uses the page title)").fill("New site");
  await page.getByPlaceholder("https://example.com", { exact: true }).fill("https://new.example.com");
  await page.getByRole("button", { name: "Add", exact: true }).click();
  await expect(page.getByText("Unchecked", { exact: true })).toHaveCount(3);
  const beforeReload = listRequests();
  await page.clock.fastForward(5000);
  await expect.poll(listRequests).toBeGreaterThan(beforeReload);
  expect(checks).toEqual([]);
  await expect(page.getByText("Unchecked", { exact: true })).toHaveCount(3);
});

test("single and all health checks run only from their manual buttons", async ({ page }) => {
  const { checks } = await setup(page);
  await page.getByRole("button", { name: "Recheck health", exact: true }).first().focus();
  await page.keyboard.press("Enter");
  await expect(page.getByText(/^Up · checked /)).toHaveCount(1);
  await expect(page.getByText("Unchecked", { exact: true })).toHaveCount(1);
  expect(checks).toEqual(["/api/bookmarks/1/check"]);

  await page.getByRole("button", { name: "Check all", exact: true }).click();
  await expect(page.getByText(/^Up · checked /)).toHaveCount(2);
  await expect(page.getByText("Unchecked", { exact: true })).toHaveCount(0);
  expect(checks).toEqual(["/api/bookmarks/1/check", "/api/bookmarks/check"]);
});
