import { useEffect, useRef, useState } from "react";
import { api, ApiError, cn } from "@/lib/api";
import { Card, Badge, PageHeader, EmptyState, Skeleton, Modal, Spinner } from "@/components/ui";
import { useTheme } from "@/lib/theme";
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  CartesianGrid,
  ReferenceLine,
} from "recharts";
import {
  Server,
  Cpu,
  MemoryStick,
  HardDrive,
  Network,
  Activity,
  Clock,
  Container as ContainerIcon,
  ChevronRight,
  Gauge,
  type LucideIcon,
} from "lucide-react";

type Process = { pid: number; name: string; cpu: number; mem_rss: number; mem_perc: number };
type Container = { id: string; name: string; image: string; state: string };
type Disk = { path: string; fstype: string; total: number; used: number; used_percent: number };
type ProcessDetail = {
  pid: number;
  ppid: number;
  name: string;
  exe: string;
  cmdline: string;
  user: string;
  status: string;
  created_at: string;
  num_threads: number;
  cpu: number;
  mem_rss: number;
  mem_perc: number;
};
type ContainerDetail = {
  id: string;
  name: string;
  image: string;
  state: string;
  status: string;
  created_at: string;
  started_at: string | null;
  command: string;
  ports: { container_port: string; host_ip: string; host_port: string }[];
  mounts: { type: string; source: string; destination: string; read_only: boolean }[];
};
type NetRate = {
  rx_bytes_per_sec: number;
  tx_bytes_per_sec: number;
  rx_packets_per_sec: number;
  tx_packets_per_sec: number;
};
type DiskRate = {
  read_bytes_per_sec: number;
  write_bytes_per_sec: number;
  read_ops_per_sec: number;
  write_ops_per_sec: number;
};
// Linux pressure-stall information, avg10 percentages.
type Pressure = {
  cpu_some: number;
  memory_some: number;
  memory_full: number;
  io_some: number;
  io_full: number;
};
type Current = {
  cpu_percent: number;
  cpu_cores: number;
  mem_total: number;
  mem_used: number; // total − available
  disk_total: number;
  disk_used: number;
  disks: Disk[];
  disk_rate: DiskRate | null;
  net_rx: number;
  net_tx: number;
  net_rate: NetRate | null; // null until two readings exist
  load1: number;
  load5: number;
  load15: number;
  pressure: Pressure | null; // null off Linux
  uptime_secs: number;
  self: { cpu: number; mem_rss: number } | null; // this Nexus process
  top_procs: Process[];
  docker: { enabled: boolean; containers: Container[] };
};
// The nullable fields are absent on samples taken before they were recorded,
// off Linux (psi_*), or without a usable baseline (disk/network rates).
type Sample = {
  taken_at: string;
  cpu_percent: number;
  mem_total: number;
  mem_used: number;
  disk_total: number;
  disk_used: number;
  load1: number;
  net_rx_rate: number | null;
  net_tx_rate: number | null;
  disk_read_rate: number | null;
  disk_write_rate: number | null;
  disk_read_iops: number | null;
  disk_write_iops: number | null;
  psi_cpu_some: number | null;
  psi_mem_some: number | null;
  psi_io_some: number | null;
  self_cpu: number | null;
  self_rss: number | null;
};

function bytes(n: number): string {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(1)} ${units[i]}`;
}

const rate = (n: number) => `${bytes(n)}/s`;
const pps = (n: number) => (n < 10 ? n.toFixed(1) : Math.round(n).toLocaleString());
const round = (n: number | null, digits = 1) => (n == null ? null : Number(n.toFixed(digits)));

function uptime(secs: number): string {
  const d = Math.floor(secs / 86400);
  const h = Math.floor((secs % 86400) / 3600);
  const m = Math.floor((secs % 3600) / 60);
  return `${d}d ${h}h ${m}m`;
}

function Stat({
  label,
  value,
  sub,
  title,
  icon: Icon,
}: {
  label: string;
  value: string;
  sub?: string;
  title?: string;
  icon: LucideIcon;
}) {
  return (
    <Card className="p-4" title={title}>
      <div className="flex items-center gap-2 text-muted-foreground">
        <Icon size={15} />
        <span className="text-xs font-medium uppercase tracking-wide">{label}</span>
      </div>
      <div className="mt-2 text-xl font-semibold tabular-nums">{value}</div>
      {sub && <div className="mt-0.5 text-xs tabular-nums text-muted-foreground">{sub}</div>}
    </Card>
  );
}

function DiskIOStat({ value }: { value: DiskRate | null }) {
  return (
    <Card className="min-w-0 p-4" aria-label="Disk I/O statistics">
      <div className="flex items-center gap-2 text-muted-foreground">
        <HardDrive size={15} aria-hidden />
        <span className="text-xs font-medium uppercase tracking-wide">Disk I/O</span>
      </div>
      {value ? (
        <dl className="mt-2 space-y-1">
          {[
            { label: "Read", bytes: value.read_bytes_per_sec, ops: value.read_ops_per_sec },
            { label: "Write", bytes: value.write_bytes_per_sec, ops: value.write_ops_per_sec },
          ].map((item) => (
            <div key={item.label} className="flex flex-wrap items-baseline justify-between gap-x-2">
              <dt className="text-xs text-muted-foreground">{item.label}</dt>
              <dd className="text-right tabular-nums">
                <span className="block text-base font-semibold">{rate(item.bytes)}</span>
                <span className="block text-[11px] text-muted-foreground">{pps(item.ops)} IOPS</span>
              </dd>
            </div>
          ))}
        </dl>
      ) : (
        <div className="mt-2">
          <div className="text-xl font-semibold">No data</div>
          <p className="mt-0.5 text-xs text-muted-foreground">Waiting for usable disk counters.</p>
        </div>
      )}
    </Card>
  );
}

const stateVariant = (s: string) =>
  s === "running" ? "success" : s === "exited" || s === "dead" ? "destructive" : "warning";

// Trend-chart windows. Each asks the server to average samples into at most
// `points` buckets (display-time only; stored samples are untouched), and
// picks an axis/tooltip label suited to its span.
type RangeKey = "1h" | "24h" | "7d" | "30d";
const ranges: { key: RangeKey; label: string; ms: number; points: number }[] = [
  { key: "1h", label: "1h", ms: 3600_000, points: 120 },
  { key: "24h", label: "24h", ms: 24 * 3600_000, points: 288 },
  { key: "7d", label: "7d", ms: 7 * 24 * 3600_000, points: 336 },
  { key: "30d", label: "30d", ms: 30 * 24 * 3600_000, points: 360 },
];
const RANGE_KEY = "nexus-servers-range";

function loadRange(): RangeKey {
  if (typeof window === "undefined") return "24h";
  const v = window.localStorage.getItem(RANGE_KEY);
  return ranges.some((r) => r.key === v) ? (v as RangeKey) : "24h";
}

function timeLabel(iso: string, range: RangeKey): string {
  const d = new Date(iso);
  if (range === "1h" || range === "24h") {
    return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  }
  return d.toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    ...(range === "7d" ? { hour: "2-digit", minute: "2-digit" } : {}),
  });
}

export default function Servers() {
  const [cur, setCur] = useState<Current | null>(null);
  const [samples, setSamples] = useState<Sample[]>([]);
  const [loading, setLoading] = useState(true);
  const [range, setRange] = useState<RangeKey>(loadRange);
  const [proc, setProc] = useState<number | null>(null);
  const [container, setContainer] = useState<string | null>(null);
  const rangeRef = useRef(range);
  rangeRef.current = range;

  async function loadCurrent() {
    setCur(await api.get<Current>("/api/servers/current"));
  }
  async function loadSamples() {
    const r = ranges.find((x) => x.key === rangeRef.current) ?? ranges[1];
    const to = new Date();
    const from = new Date(to.getTime() - r.ms);
    const params = new URLSearchParams({
      from: from.toISOString().replace(/\.\d{3}Z$/, "Z"),
      to: to.toISOString().replace(/\.\d{3}Z$/, "Z"),
      max_points: String(r.points),
    });
    const key = r.key;
    const rows = await api.get<Sample[]>(`/api/servers/samples?${params}`);
    // Drop a response for a range the Owner has already switched away from.
    if (rangeRef.current === key) setSamples(rows);
  }

  useEffect(() => {
    Promise.all([loadCurrent(), loadSamples()]).finally(() => setLoading(false));
    const t1 = setInterval(loadCurrent, 5000);
    const t2 = setInterval(loadSamples, 30000);
    return () => {
      clearInterval(t1);
      clearInterval(t2);
    };
  }, []);

  function changeRange(next: RangeKey) {
    if (next === range) return;
    setRange(next);
    rangeRef.current = next;
    window.localStorage.setItem(RANGE_KEY, next);
    loadSamples().catch(() => {});
  }

  const chartData = samples.map((s) => ({
    t: timeLabel(s.taken_at, range),
    at: s.taken_at,
    cpu: round(s.cpu_percent),
    memPct: s.mem_total ? round((s.mem_used / s.mem_total) * 100) : 0,
    diskRead: round(s.disk_read_rate, 0),
    diskWrite: round(s.disk_write_rate, 0),
    rx: round(s.net_rx_rate, 0),
    tx: round(s.net_tx_rate, 0),
    load: round(s.load1, 2),
    psiCpu: round(s.psi_cpu_some, 2),
    psiMem: round(s.psi_mem_some, 2),
    psiIo: round(s.psi_io_some, 2),
    selfCpu: round(s.self_cpu, 2),
    selfRss: s.self_rss,
  }));
  const hasPressure = cur?.pressure != null || samples.some((s) => s.psi_cpu_some != null);
  const hasSelf = cur?.self != null || samples.some((s) => s.self_rss != null);
  const cores = cur?.cpu_cores ?? 0;

  return (
    <div className="space-y-6">
      <PageHeader
        icon={Server}
        title="Servers"
        description="Live vitals for this Host, refreshed every few seconds."
        actions={
          <div className="flex items-center gap-3">
            {cur?.self && (
              <span
                className="text-xs tabular-nums text-muted-foreground"
                title="This Nexus process's own CPU (% of one core) and resident memory"
              >
                Nexus {cur.self.cpu.toFixed(1)}% · {bytes(cur.self.mem_rss)}
              </span>
            )}
            <Badge variant="success">
              <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-success" />
              Live
            </Badge>
          </div>
        }
      />

      {loading ? (
        <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-6">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-[6.5rem]" />
          ))}
        </div>
      ) : cur ? (
        <>
          <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-6">
            <Stat
              icon={Cpu}
              label="CPU"
              value={`${cur.cpu_percent.toFixed(1)}%`}
              sub={
                cores > 0
                  ? `${((cur.cpu_percent / 100) * cores).toFixed(1)} of ${cores} cores`
                  : undefined
              }
            />
            <Stat
              icon={MemoryStick}
              label="Memory"
              value={`${bytes(cur.mem_used)} / ${bytes(cur.mem_total)}`}
              sub={`${bytes(Math.max(0, cur.mem_total - cur.mem_used))} available`}
              title="Used = total − available; reclaimable page cache doesn't count."
            />
            <DiskIOStat value={cur.disk_rate} />
            {cur.net_rate ? (
              <Stat
                icon={Network}
                label="Network"
                value={`↓${rate(cur.net_rate.rx_bytes_per_sec)} ↑${rate(cur.net_rate.tx_bytes_per_sec)}`}
                sub={`${pps(cur.net_rate.rx_packets_per_sec)} / ${pps(cur.net_rate.tx_packets_per_sec)} pkt/s`}
                title={`Since boot: ↓${bytes(cur.net_rx)} ↑${bytes(cur.net_tx)} (loopback excluded)`}
              />
            ) : (
              <Stat
                icon={Network}
                label="Network"
                value={`↓${bytes(cur.net_rx)} ↑${bytes(cur.net_tx)}`}
                sub="since boot"
              />
            )}
            <Stat
              icon={Activity}
              label="Load"
              value={`${cur.load1.toFixed(2)} ${cur.load5.toFixed(2)} ${cur.load15.toFixed(2)}`}
              sub={cores > 0 ? `${(cur.load1 / cores).toFixed(2)} per core` : undefined}
              title="1, 5 and 15-minute load averages; above 1.00 per core means work is queueing."
            />
            <Stat icon={Clock} label="Uptime" value={uptime(cur.uptime_secs)} />
          </div>

          {(cur.disks.length > 0 || cur.pressure) && (
            <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
              {cur.disks.length > 0 && (
                <DisksCard
                  disks={cur.disks}
                  className={cur.pressure ? "lg:col-span-2" : "lg:col-span-3"}
                />
              )}
              {cur.pressure && (
                <PressureCard
                  pressure={cur.pressure}
                  className={cur.disks.length > 0 ? undefined : "lg:col-span-3"}
                />
              )}
            </div>
          )}

          <div className="flex flex-wrap items-center justify-between gap-2">
            <h2 className="text-sm font-semibold text-muted-foreground">Trends</h2>
            <div
              className="inline-flex rounded-md border border-border p-0.5"
              role="group"
              aria-label="Chart time range"
            >
              {ranges.map((r) => (
                <button
                  key={r.key}
                  type="button"
                  aria-pressed={range === r.key}
                  onClick={() => changeRange(r.key)}
                  className={cn(
                    "rounded px-2.5 py-1 text-xs font-medium tabular-nums transition-colors",
                    range === r.key
                      ? "bg-primary text-primary-foreground"
                      : "text-muted-foreground hover:bg-muted hover:text-foreground",
                  )}
                >
                  {r.label}
                </button>
              ))}
            </div>
          </div>

          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Chart title="CPU %" data={chartData} series={[{ key: "cpu", label: "CPU" }]} unit="%" />
            <Chart
              title="Memory %"
              data={chartData}
              series={[{ key: "memPct", label: "Memory" }]}
              unit="%"
            />
            <Chart
              title="Disk I/O throughput"
              data={chartData}
              series={[
                { key: "diskRead", label: "Read" },
                { key: "diskWrite", label: "Write", tone: "orange", dashed: true },
              ]}
              format={rate}
              axisWidth={84}
              noDataMessage="No disk I/O history yet. New readings will appear after sampling."
              showDataTable
            />
            <Chart
              title="Network throughput"
              data={chartData}
              series={[
                { key: "rx", label: "↓ in" },
                { key: "tx", label: "↑ out", tone: "orange" },
              ]}
              format={rate}
              axisWidth={64}
            />
            <Chart
              title="Load (1 min)"
              data={chartData}
              series={[{ key: "load", label: "Load" }]}
              refLine={cores > 0 ? { y: cores, label: `${cores} cores` } : undefined}
            />
            {hasPressure && (
              <Chart
                title="Pressure stall (some, avg10)"
                data={chartData}
                series={[
                  { key: "psiCpu", label: "CPU" },
                  { key: "psiMem", label: "Memory", tone: "orange" },
                  { key: "psiIo", label: "I/O", tone: "violet" },
                ]}
                unit="%"
              />
            )}
            {hasSelf && (
              <Chart
                title="Nexus process"
                data={chartData}
                series={[
                  { key: "selfRss", label: "Memory" },
                  { key: "selfCpu", label: "CPU", tone: "orange", right: true },
                ]}
                format={bytes}
                rightUnit="%"
                axisWidth={56}
              />
            )}
          </div>

          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Card className="p-4">
              <div className="mb-3 flex items-center gap-2">
                <Activity size={16} className="text-muted-foreground" />
                <h2 className="font-semibold">Top processes</h2>
              </div>
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-border text-left text-xs text-muted-foreground">
                      <th className="pb-2 font-medium">PID</th>
                      <th className="pb-2 font-medium">Name</th>
                      <th className="pb-2 text-right font-medium">CPU%</th>
                      <th className="pb-2 text-right font-medium">Mem</th>
                      <th className="pb-2" aria-hidden />
                    </tr>
                  </thead>
                  <tbody>
                    {cur.top_procs.map((p) => (
                      <tr
                        key={p.pid}
                        tabIndex={0}
                        onClick={() => setProc(p.pid)}
                        onKeyDown={(e) => {
                          if (e.key === "Enter" || e.key === " ") {
                            e.preventDefault();
                            setProc(p.pid);
                          }
                        }}
                        title="Show process details"
                        className="group cursor-pointer border-b border-border/60 outline-none transition-colors last:border-0 hover:bg-muted/50 focus-visible:bg-muted/50"
                      >
                        <td className="py-1.5 font-mono text-xs text-muted-foreground">
                          {p.pid}
                        </td>
                        <td className="py-1.5">{p.name}</td>
                        <td className="py-1.5 text-right tabular-nums">
                          {p.cpu.toFixed(1)}
                        </td>
                        <td className="py-1.5 text-right tabular-nums">
                          {bytes(p.mem_rss)}
                        </td>
                        <td className="w-5 py-1.5 text-right">
                          <ChevronRight
                            size={14}
                            className="inline text-muted-foreground/40 transition group-hover:text-muted-foreground"
                          />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </Card>

            <Card className="p-4">
              <div className="mb-3 flex items-center gap-2">
                <ContainerIcon size={16} className="text-muted-foreground" />
                <h2 className="font-semibold">Docker</h2>
              </div>
              {!cur.docker.enabled ? (
                <EmptyState
                  icon={ContainerIcon}
                  title="Docker not enabled"
                  description="No container runtime is being monitored on this Host."
                  className="py-8"
                />
              ) : cur.docker.containers.length === 0 ? (
                <EmptyState
                  icon={ContainerIcon}
                  title="No containers"
                  className="py-8"
                />
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-sm">
                    <thead>
                      <tr className="border-b border-border text-left text-xs text-muted-foreground">
                        <th className="pb-2 font-medium">Name</th>
                        <th className="pb-2 font-medium">Image</th>
                        <th className="pb-2 font-medium">State</th>
                        <th className="pb-2" aria-hidden />
                      </tr>
                    </thead>
                    <tbody>
                      {cur.docker.containers.map((c) => (
                        <tr
                          key={c.id}
                          tabIndex={0}
                          onClick={() => setContainer(c.id)}
                          onKeyDown={(e) => {
                            if (e.key === "Enter" || e.key === " ") {
                              e.preventDefault();
                              setContainer(c.id);
                            }
                          }}
                          title="Show container details"
                          className="group cursor-pointer border-b border-border/60 outline-none transition-colors last:border-0 hover:bg-muted/50 focus-visible:bg-muted/50"
                        >
                          <td className="py-1.5 font-medium">{c.name}</td>
                          <td className="py-1.5 font-mono text-xs text-muted-foreground">
                            {c.image}
                          </td>
                          <td className="py-1.5">
                            <Badge variant={stateVariant(c.state)}>{c.state}</Badge>
                          </td>
                          <td className="w-5 py-1.5 text-right">
                            <ChevronRight
                              size={14}
                              className="inline text-muted-foreground/40 transition group-hover:text-muted-foreground"
                            />
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </Card>
          </div>
        </>
      ) : (
        <EmptyState
          icon={Server}
          title="No data"
          description="Could not read Host vitals."
        />
      )}

      {proc != null && <ProcessModal pid={proc} onClose={() => setProc(null)} />}
      {container != null && (
        <ContainerModal id={container} onClose={() => setContainer(null)} />
      )}
    </div>
  );
}

function DisksCard({ disks, className }: { disks: Disk[]; className?: string }) {
  return (
    <Card className={cn("p-4", className)}>
      <div className="mb-3 flex items-center gap-2">
        <HardDrive size={16} className="text-muted-foreground" />
        <h2 className="font-semibold">Filesystems</h2>
        <span className="text-xs text-muted-foreground">current usage per mount</span>
      </div>
      <ul className="grid grid-cols-1 gap-x-6 gap-y-3 md:grid-cols-2">
        {disks.map((d) => {
          const pct = d.used_percent;
          const tone = pct >= 90 ? "bg-destructive" : pct >= 75 ? "bg-warning" : "bg-primary";
          return (
            <li key={d.path} className="min-w-0">
              <div className="flex items-baseline gap-2 text-sm">
                <span className="truncate font-mono text-xs font-medium" title={d.path}>
                  {d.path}
                </span>
                <span className="text-[11px] text-muted-foreground">{d.fstype}</span>
                <span className="ml-auto shrink-0 text-xs tabular-nums text-muted-foreground">
                  {bytes(d.used)} / {bytes(d.total)}
                  <span
                    className={cn(
                      "ml-1.5 font-medium",
                      pct >= 90 ? "text-destructive" : "text-foreground",
                    )}
                  >
                    {pct.toFixed(0)}%
                  </span>
                </span>
              </div>
              <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-muted">
                <div
                  className={cn("h-full rounded-full", tone)}
                  style={{ width: `${Math.min(100, pct)}%` }}
                />
              </div>
            </li>
          );
        })}
      </ul>
    </Card>
  );
}

// PressureCard shows Linux pressure-stall information: the share of the last
// 10 seconds in which some (or, for "full", all) non-idle tasks were stalled
// waiting on CPU, memory or I/O. Unlike utilisation it measures lost time
// directly, so any sustained non-zero value is worth a look.
function PressureCard({ pressure: p, className }: { pressure: Pressure; className?: string }) {
  const rows: { label: string; some: number; full?: number }[] = [
    { label: "CPU", some: p.cpu_some },
    { label: "Memory", some: p.memory_some, full: p.memory_full },
    { label: "I/O", some: p.io_some, full: p.io_full },
  ];
  return (
    <Card className={cn("p-4", className)}>
      <div className="mb-3 flex items-center gap-2">
        <Gauge size={16} className="text-muted-foreground" />
        <h2 className="font-semibold">Pressure</h2>
        <span className="text-xs text-muted-foreground">% of last 10s stalled</span>
      </div>
      <ul className="space-y-3">
        {rows.map((r) => {
          const tone = r.some >= 40 ? "bg-destructive" : r.some >= 10 ? "bg-warning" : "bg-primary";
          return (
            <li key={r.label}>
              <div className="flex items-baseline gap-2 text-sm">
                <span className="text-xs font-medium">{r.label}</span>
                <span className="ml-auto text-xs tabular-nums text-muted-foreground">
                  some{" "}
                  <span
                    className={cn("font-medium", r.some >= 40 ? "text-destructive" : "text-foreground")}
                  >
                    {r.some.toFixed(2)}%
                  </span>
                  {r.full != null && (
                    <>
                      {" · "}full <span className="font-medium text-foreground">{r.full.toFixed(2)}%</span>
                    </>
                  )}
                </span>
              </div>
              <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-muted">
                <div
                  className={cn("h-full rounded-full", tone)}
                  style={{ width: `${Math.min(100, r.some)}%` }}
                />
              </div>
            </li>
          );
        })}
      </ul>
    </Card>
  );
}

// useDetail fetches a read-only drill-in, distinguishing "gone" (404) and
// "unavailable" (503) from other failures so the modal can say which.
function useDetail<T>(path: string) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<{ status: number; message: string } | null>(null);
  useEffect(() => {
    let alive = true;
    api
      .get<T>(path)
      .then((d) => alive && setData(d))
      .catch((err) => {
        if (!alive) return;
        const status = err instanceof ApiError ? err.status : 0;
        setError({ status, message: (err as Error).message });
      });
    return () => {
      alive = false;
    };
  }, [path]);
  return { data, error };
}

function DetailRows({ rows }: { rows: [string, React.ReactNode][] }) {
  return (
    <dl className="grid grid-cols-[7rem_1fr] gap-x-3 gap-y-2 text-sm">
      {rows.map(([k, v]) => (
        <div key={k} className="contents">
          <dt className="text-xs font-medium text-muted-foreground">{k}</dt>
          <dd className="min-w-0 break-words">{v === "" || v == null ? "—" : v}</dd>
        </div>
      ))}
    </dl>
  );
}

function DetailState({
  error,
  goneTitle,
}: {
  error: { status: number; message: string };
  goneTitle: string;
}) {
  if (error.status === 404) {
    return (
      <EmptyState
        title={goneTitle}
        description="It went away after this list was refreshed."
        className="py-8"
      />
    );
  }
  return <EmptyState title="Details unavailable" description={error.message} className="py-8" />;
}

function ProcessModal({ pid, onClose }: { pid: number; onClose: () => void }) {
  const { data: d, error } = useDetail<ProcessDetail>(`/api/servers/processes/${pid}`);
  return (
    <Modal
      open
      onClose={onClose}
      icon={Activity}
      title={d ? d.name || `PID ${pid}` : `PID ${pid}`}
      description="Read-only process details."
      className="sm:max-w-xl"
    >
      {error ? (
        <DetailState error={error} goneTitle="Process no longer running" />
      ) : !d ? (
        <div className="flex justify-center py-8">
          <Spinner />
        </div>
      ) : (
        <div className="space-y-4">
          {d.cmdline && (
            <pre className="max-h-32 overflow-auto whitespace-pre-wrap break-all rounded-md border border-border bg-muted/40 p-2.5 font-mono text-xs">
              {d.cmdline}
            </pre>
          )}
          <DetailRows
            rows={[
              ["PID / parent", <span className="font-mono text-xs">{d.pid} / {d.ppid}</span>],
              ["User", d.user],
              ["Status", d.status],
              ["Started", d.created_at ? new Date(d.created_at).toLocaleString() : ""],
              ["Threads", d.num_threads],
              ["CPU", `${d.cpu.toFixed(1)}%`],
              ["Memory", `${bytes(d.mem_rss)} (${d.mem_perc.toFixed(1)}%)`],
              ["Executable", <span className="font-mono text-xs">{d.exe}</span>],
            ]}
          />
        </div>
      )}
    </Modal>
  );
}

function ContainerModal({ id, onClose }: { id: string; onClose: () => void }) {
  const { data: d, error } = useDetail<ContainerDetail>(
    `/api/servers/containers/${encodeURIComponent(id)}`,
  );
  return (
    <Modal
      open
      onClose={onClose}
      icon={ContainerIcon}
      title={d ? d.name.replace(/^\//, "") : id}
      description="Read-only container details."
      className="sm:max-w-xl"
    >
      {error ? (
        <DetailState error={error} goneTitle="Container no longer present" />
      ) : !d ? (
        <div className="flex justify-center py-8">
          <Spinner />
        </div>
      ) : (
        <div className="space-y-4">
          <DetailRows
            rows={[
              ["ID", <span className="font-mono text-xs">{d.id}</span>],
              ["Image", <span className="font-mono text-xs">{d.image}</span>],
              [
                "State",
                <span className="inline-flex items-center gap-2">
                  <Badge variant={stateVariant(d.state)}>{d.state}</Badge>
                  <span className="text-xs text-muted-foreground">{d.status}</span>
                </span>,
              ],
              ["Created", new Date(d.created_at).toLocaleString()],
              ["Started", d.started_at ? new Date(d.started_at).toLocaleString() : ""],
              ["Command", <span className="font-mono text-xs">{d.command}</span>],
              [
                "Ports",
                d.ports.length === 0 ? (
                  ""
                ) : (
                  <ul className="space-y-0.5 font-mono text-xs">
                    {d.ports.map((p, i) => (
                      <li key={i}>
                        {p.host_ip || "*"}:{p.host_port} → {p.container_port}
                      </li>
                    ))}
                  </ul>
                ),
              ],
              [
                "Mounts",
                d.mounts.length === 0 ? (
                  ""
                ) : (
                  <ul className="space-y-0.5 font-mono text-xs">
                    {d.mounts.map((m, i) => (
                      <li key={i} className="break-all">
                        {m.source} → {m.destination}
                        <span className="ml-1 text-muted-foreground">
                          ({m.type}
                          {m.read_only ? ", ro" : ""})
                        </span>
                      </li>
                    ))}
                  </ul>
                ),
              ],
            ]}
          />
        </div>
      )}
    </Modal>
  );
}

// Series tones: the brand teal first, then two distinguishable accents.
const tones = {
  teal: { light: "#0d9488", dark: "#2dd4bf" },
  orange: { light: "#ea580c", dark: "#fb923c" },
  violet: { light: "#7c3aed", dark: "#a78bfa" },
};
type Series = { key: string; label: string; tone?: keyof typeof tones; right?: boolean; dashed?: boolean };

// Chart plots one or more series over the samples. Null points (a sample with
// no reading for that series) leave a gap rather than drawing a false zero.
// Series marked `right` use a second axis labelled with rightUnit.
function Chart({
  title,
  data,
  series,
  unit,
  format,
  rightUnit,
  refLine,
  axisWidth = 44,
  noDataMessage,
  showDataTable = false,
}: {
  title: string;
  data: Record<string, number | string | null>[];
  series: Series[];
  unit?: string;
  format?: (n: number) => string;
  rightUnit?: string;
  refLine?: { y: number; label: string };
  axisWidth?: number;
  noDataMessage?: string;
  showDataTable?: boolean;
}) {
  const { theme } = useTheme();
  const grid = theme === "dark" ? "#1e293b" : "#e2e8f0";
  const axis = theme === "dark" ? "#64748b" : "#94a3b8";
  const color = (s: Series) => tones[s.tone ?? "teal"][theme === "dark" ? "dark" : "light"];
  const fmt = (v: number) => (format ? format(v) : `${v}${unit ?? ""}`);
  const fmtRight = (v: number) => `${v}${rightUnit ?? ""}`;
  const hasRight = series.some((s) => s.right);
  const hasData = data.some((row) => series.some((s) => row[s.key] != null));

  return (
    <Card className="min-w-0 p-4" aria-label={title}>
      <div className="mb-3 flex flex-wrap items-center gap-3">
        <h2 className="text-sm font-semibold text-muted-foreground">{title}</h2>
        {series.length > 1 && (
          <div className="ml-auto flex items-center gap-3 text-[11px] text-muted-foreground">
            {series.map((s) => (
              <span key={s.key} className="inline-flex items-center gap-1">
                <span
                  aria-hidden
                  className={s.dashed ? "w-3 border-t-2 border-dashed" : "w-3 border-t-2"}
                  style={{ borderColor: color(s) }}
                />
                {s.label}
              </span>
            ))}
          </div>
        )}
      </div>
      {!hasData && noDataMessage ? (
        <div role="status" className="flex min-h-[180px] items-center justify-center px-3 text-center text-sm text-muted-foreground">
          {noDataMessage}
        </div>
      ) : <ResponsiveContainer width="100%" height={180}>
        <AreaChart data={data} accessibilityLayer margin={{ top: 4, right: 4, left: 0, bottom: 0 }}>
          <defs>
            {series.map((s) => (
              <linearGradient key={s.key} id={`grad-${s.key}`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={color(s)} stopOpacity={series.length > 1 ? 0.15 : 0.25} />
                <stop offset="100%" stopColor={color(s)} stopOpacity={0} />
              </linearGradient>
            ))}
          </defs>
          <CartesianGrid strokeDasharray="3 3" stroke={grid} vertical={false} />
          <XAxis
            dataKey="t"
            tick={{ fill: axis, fontSize: 10 }}
            axisLine={false}
            tickLine={false}
            minTickGap={32}
          />
          <YAxis
            yAxisId="left"
            width={axisWidth}
            tick={{ fill: axis, fontSize: 11 }}
            axisLine={false}
            tickLine={false}
            tickFormatter={(v) => fmt(Number(v))}
          />
          {hasRight && (
            <YAxis
              yAxisId="right"
              orientation="right"
              width={40}
              tick={{ fill: axis, fontSize: 11 }}
              axisLine={false}
              tickLine={false}
              tickFormatter={(v) => fmtRight(Number(v))}
            />
          )}
          {refLine && (
            <ReferenceLine
              yAxisId="left"
              y={refLine.y}
              stroke={axis}
              strokeDasharray="4 4"
              label={{ value: refLine.label, position: "insideTopRight", fill: axis, fontSize: 10 }}
            />
          )}
          <Tooltip
            contentStyle={{
              background: "hsl(var(--card))",
              border: "1px solid hsl(var(--border))",
              borderRadius: 8,
              fontSize: 12,
              color: "hsl(var(--card-foreground))",
            }}
            labelStyle={{ color: "hsl(var(--muted-foreground))" }}
            formatter={(v: number, _name, item) => {
              const s = series.find((x) => x.key === item.dataKey);
              return [s?.right ? fmtRight(v) : fmt(v), s?.label ?? title];
            }}
          />
          {series.map((s) => (
            <Area
              key={s.key}
              yAxisId={s.right ? "right" : "left"}
              type="monotone"
              dataKey={s.key}
              name={s.label}
              stroke={color(s)}
              strokeWidth={2}
              strokeDasharray={s.dashed ? "5 3" : undefined}
              fill={`url(#grad-${s.key})`}
              dot={false}
              isAnimationActive={false}
            />
          ))}
        </AreaChart>
      </ResponsiveContainer>}
      {showDataTable && hasData && (
        <details className="mt-3 border-t border-border pt-2">
          <summary className="cursor-pointer rounded text-xs text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
            View disk I/O data
          </summary>
          <div className="mt-2 max-h-56 overflow-auto">
            <table className="w-full text-xs">
              <caption className="sr-only">Disk I/O history in bytes per second</caption>
              <thead>
                <tr className="text-left text-muted-foreground">
                  <th scope="col" className="pb-1 font-medium">Time</th>
                  {series.map((s) => <th key={s.key} scope="col" className="pb-1 text-right font-medium">{s.label}</th>)}
                </tr>
              </thead>
              <tbody>
                {data.map((row, i) => (
                  <tr key={i} className="border-t border-border/60">
                    <th scope="row" className="py-1 text-left font-normal">{new Date(String(row.at)).toLocaleString()}</th>
                    {series.map((s) => <td key={s.key} className="py-1 text-right tabular-nums">{row[s.key] == null ? "—" : fmt(Number(row[s.key]))}</td>)}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </details>
      )}
    </Card>
  );
}
