import { test, expect, type Page } from "@playwright/test";

const diskRate = {
  read_bytes_per_sec: 2 * 1024 * 1024,
  write_bytes_per_sec: 1024 * 1024,
  read_ops_per_sec: 120,
  write_ops_per_sec: 60,
};

async function setup(page: Page, options: { noData?: boolean; idle?: boolean; legacy?: boolean; warmup?: boolean } = {}) {
  const requests: URL[] = [];
  let diskAvailable = !options.noData && !options.warmup;
  const rate = options.idle ? { read_bytes_per_sec: 0, write_bytes_per_sec: 0, read_ops_per_sec: 0, write_ops_per_sec: 0 } : diskRate;
  const now = Date.now();
  const samples = Array.from({ length: 12 }, (_, i) => ({
    taken_at: new Date(now - (12 - i) * 150_000).toISOString(),
    cpu_percent: 20 + i * 2, mem_total: 16 * 1024 ** 3, mem_used: 8 * 1024 ** 3,
    disk_total: 512 * 1024 ** 3, disk_used: 100 * 1024 ** 3, load1: 1,
    net_rx_rate: 1024, net_tx_rate: 512,
    psi_cpu_some: null, psi_mem_some: null, psi_io_some: null,
    self_cpu: null, self_rss: null,
    disk_read_rate: options.legacy || i === 0 ? null : rate.read_bytes_per_sec,
    disk_write_rate: options.legacy || i === 0 ? null : rate.write_bytes_per_sec,
    disk_read_iops: options.legacy || i === 0 ? null : rate.read_ops_per_sec,
    disk_write_iops: options.legacy || i === 0 ? null : rate.write_ops_per_sec,
  }));
  await page.route("**/api/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname === "/api/auth/me") return route.fulfill({ json: { authenticated: true } });
    if (url.pathname === "/api/servers/current") {
      return route.fulfill({ json: {
        cpu_percent: 28, cpu_cores: 8, mem_total: 16 * 1024 ** 3, mem_used: 8 * 1024 ** 3,
        disk_total: 512 * 1024 ** 3, disk_used: 100 * 1024 ** 3,
        disks: [{ path: "/", fstype: "apfs", total: 512 * 1024 ** 3, used: 100 * 1024 ** 3, used_percent: 19.5 }],
        disk_rate: diskAvailable ? rate : null,
        net_rx: 10 * 1024 ** 3, net_tx: 2 * 1024 ** 3,
        net_rate: { rx_bytes_per_sec: 1024, tx_bytes_per_sec: 512, rx_packets_per_sec: 5, tx_packets_per_sec: 3 },
        load1: 1, load5: 0.9, load15: 0.8, uptime_secs: 86400,
        pressure: null, self: null, top_procs: [], docker: { enabled: false, containers: [] },
      } });
    }
    if (url.pathname === "/api/servers/samples") {
      requests.push(url);
      return route.fulfill({ json: samples });
    }
    return route.fulfill({ status: 404, json: { error: "not found" } });
  });
  await page.goto("/servers");
  await expect(page.locator('[aria-label="Disk I/O statistics"]')).toBeVisible();
  await expect(page.getByRole("heading", { name: "Disk I/O throughput", exact: true })).toBeVisible();
  return { requests, enableDiskRate: () => { diskAvailable = true; } };
}

test("Disk shows read/write throughput and IOPS; capacity stays in Filesystems", async ({ page }) => {
  await setup(page);
  const stat = page.locator('[aria-label="Disk I/O statistics"]');
  await expect(stat.getByText("Read", { exact: true })).toBeVisible();
  await expect(stat.getByText("Write", { exact: true })).toBeVisible();
  await expect(stat.getByText("2.0 MB/s", { exact: true })).toBeVisible();
  await expect(stat.getByText("1.0 MB/s", { exact: true })).toBeVisible();
  await expect(stat.getByText("120 IOPS", { exact: true })).toBeVisible();
  await expect(stat.getByText("60 IOPS", { exact: true })).toBeVisible();
  await expect(page.getByRole("heading", { name: "Disk %", exact: true })).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "Filesystems", exact: true })).toBeVisible();
  await expect(page.getByText("100.0 GB / 512.0 GB")).toBeVisible();
  const chart = page.locator('[aria-label="Disk I/O throughput"]');
  await expect(chart.locator(".recharts-area-curve")).toHaveCount(2);
  await expect(chart.locator('.recharts-area-curve[stroke-dasharray="5 3"]')).toHaveCount(1);
  await expect(chart.locator("span").filter({ hasText: /^Read$/ })).toBeVisible();
  await expect(chart.locator("span").filter({ hasText: /^Write$/ })).toBeVisible();
});

test("unavailable counters and legacy history do not appear as measured zero", async ({ page }) => {
  await setup(page, { noData: true, legacy: true });
  const stat = page.locator('[aria-label="Disk I/O statistics"]');
  await expect(stat.getByText("No data", { exact: true })).toBeVisible();
  await expect(stat.getByText("0.0 B/s", { exact: true })).toHaveCount(0);
  const chart = page.locator('[aria-label="Disk I/O throughput"]');
  await expect(chart.getByRole("status")).toContainText("No disk I/O history yet");
  await expect(chart.locator(".recharts-area-curve")).toHaveCount(0);
});

test("measured idle disks display zero rates and remain chartable", async ({ page }) => {
  await setup(page, { idle: true });
  const stat = page.locator('[aria-label="Disk I/O statistics"]');
  await expect(stat.getByText("0.0 B/s", { exact: true })).toHaveCount(2);
  await expect(stat.getByText("0.0 IOPS", { exact: true })).toHaveCount(2);
  await expect(stat.getByText("No data", { exact: true })).toHaveCount(0);
  await expect(page.locator('[aria-label="Disk I/O throughput"] .recharts-area-curve')).toHaveCount(2);
});

test("disk rate appears after the next live polling observation", async ({ page }) => {
  await page.clock.install();
  const { enableDiskRate } = await setup(page, { warmup: true });
  const stat = page.locator('[aria-label="Disk I/O statistics"]');
  await expect(stat.getByText("No data", { exact: true })).toBeVisible();
  enableDiskRate();
  await page.clock.fastForward(5000);
  await expect(stat.getByText("2.0 MB/s", { exact: true })).toBeVisible();
  await expect(stat.getByText("No data", { exact: true })).toHaveCount(0);
});

test("historical read/write values are keyboard accessible with gaps for missing data", async ({ page }) => {
  const { requests } = await setup(page);
  const chart = page.locator('[aria-label="Disk I/O throughput"]');
  const toggle = chart.locator("summary");
  await toggle.focus();
  await page.keyboard.press("Enter");
  const table = chart.getByRole("table");
  await expect(table).toBeVisible();
  await expect(table.getByRole("columnheader")).toHaveText(["Time", "Read", "Write"]);
  await expect(table.locator("tbody tr").first().locator("td")).toHaveText(["—", "—"]);
  await expect(table.locator("tbody tr").last().locator("td")).toHaveText(["2.0 MB/s", "1.0 MB/s"]);
  await page.getByRole("group", { name: "Chart time range" }).getByRole("button", { name: "7d", exact: true }).click();
  await expect.poll(() => requests.at(-1)?.searchParams.get("max_points")).toBe("336");
  await expect(chart.getByRole("table")).toBeVisible();
});

test("visual QA: disk I/O cards and curves remain usable on desktop and dark mobile", async ({ page }) => {
  await page.setViewportSize({ width: 1440, height: 1100 });
  await setup(page);
  await page.screenshot({ path: test.info().outputPath("servers-disk-io-desktop.png") });
  await page.getByRole("button", { name: "Switch to dark mode", exact: true }).click();
  await page.setViewportSize({ width: 375, height: 812 });
  await page.emulateMedia({ reducedMotion: "reduce", colorScheme: "dark" });
  await expect(page.locator("html")).toHaveClass(/dark/);
  await expect(page.locator('[aria-label="Disk I/O statistics"]')).toBeVisible();
  await expect(page.locator('[aria-label="Disk I/O statistics"]').getByText("2.0 MB/s", { exact: true })).toBeVisible();
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  await page.screenshot({ path: test.info().outputPath("servers-disk-io-mobile-dark.png"), fullPage: true });
  await page.setViewportSize({ width: 812, height: 375 });
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});
