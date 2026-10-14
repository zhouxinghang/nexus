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
type Current = {
  cpu_percent: number;
  mem_total: number;
  mem_used: number;
  disk_total: number;
  disk_used: number;
  disks: Disk[];
  net_rx: number;
  net_tx: number;
  load1: number;
  load5: number;
  load15: number;
  uptime_secs: number;
  top_procs: Process[];
  docker: { enabled: boolean; containers: Container[] };
};
type Sample = {
  taken_at: string;
  cpu_percent: number;
  mem_total: number;
  mem_used: number;
  disk_total: number;
  disk_used: number;
  net_rx: number;
  net_tx: number;
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

function uptime(secs: number): string {
  const d = Math.floor(secs / 86400);
  const h = Math.floor((secs % 86400) / 3600);
  const m = Math.floor((secs % 3600) / 60);
  return `${d}d ${h}h ${m}m`;
}

function Stat({
  label,
  value,
  icon: Icon,
}: {
  label: string;
  value: string;
  icon: LucideIcon;
}) {
  return (
    <Card className="p-4">
      <div className="flex items-center gap-2 text-muted-foreground">
        <Icon size={15} />
        <span className="text-xs font-medium uppercase tracking-wide">{label}</span>
      </div>
      <div className="mt-2 text-xl font-semibold tabular-nums">{value}</div>
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
    cpu: Number(s.cpu_percent.toFixed(1)),
    memPct: s.mem_total ? Number(((s.mem_used / s.mem_total) * 100).toFixed(1)) : 0,
    diskPct: s.disk_total ? Number(((s.disk_used / s.disk_total) * 100).toFixed(1)) : 0,
    net: s.net_rx + s.net_tx,
  }));

  return (
    <div className="space-y-6">
      <PageHeader
        icon={Server}
        title="Servers"
        description="Live vitals for this Host, refreshed every few seconds."
        actions={
          <Badge variant="success">
            <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-success" />
            Live
          </Badge>
        }
      />

      {loading ? (
        <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-6">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-[5.5rem]" />
          ))}
        </div>
      ) : cur ? (
        <>
          <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-6">
            <Stat icon={Cpu} label="CPU" value={`${cur.cpu_percent.toFixed(1)}%`} />
            <Stat
              icon={MemoryStick}
              label="Memory"
              value={`${bytes(cur.mem_used)} / ${bytes(cur.mem_total)}`}
            />
            <Stat
              icon={HardDrive}
              label="Disk"
              value={`${bytes(cur.disk_used)} / ${bytes(cur.disk_total)}`}
            />
            <Stat
              icon={Network}
              label="Network"
              value={`↓${bytes(cur.net_rx)} ↑${bytes(cur.net_tx)}`}
            />
            <Stat
              icon={Activity}
              label="Load"
              value={`${cur.load1.toFixed(2)} ${cur.load5.toFixed(2)} ${cur.load15.toFixed(2)}`}
            />
            <Stat icon={Clock} label="Uptime" value={uptime(cur.uptime_secs)} />
          </div>

          {cur.disks.length > 0 && <DisksCard disks={cur.disks} />}

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
            <Chart title="CPU %" data={chartData} dataKey="cpu" unit="%" />
            <Chart title="Memory %" data={chartData} dataKey="memPct" unit="%" />
            <Chart title="Disk %" data={chartData} dataKey="diskPct" unit="%" />
            <Chart title="Network" data={chartData} dataKey="net" format={bytes} />
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

function DisksCard({ disks }: { disks: Disk[] }) {
  return (
    <Card className="p-4">
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

function Chart({
  title,
  data,
  dataKey,
  unit,
  format,
}: {
  title: string;
  data: Record<string, number | string>[];
  dataKey: string;
  unit?: string;
  format?: (n: number) => string;
}) {
  const { theme } = useTheme();
  const stroke = theme === "dark" ? "#2dd4bf" : "#0d9488";
  const grid = theme === "dark" ? "#1e293b" : "#e2e8f0";
  const axis = theme === "dark" ? "#64748b" : "#94a3b8";
  const gradId = `grad-${dataKey}`;

  return (
    <Card className="p-4">
      <h2 className="mb-3 text-sm font-semibold text-muted-foreground">{title}</h2>
      <ResponsiveContainer width="100%" height={180}>
        <AreaChart data={data} margin={{ top: 4, right: 4, left: 0, bottom: 0 }}>
          <defs>
            <linearGradient id={gradId} x1="0" y1="0" x2="0" y2="1">
              <stop offset="0%" stopColor={stroke} stopOpacity={0.25} />
              <stop offset="100%" stopColor={stroke} stopOpacity={0} />
            </linearGradient>
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
            width={44}
            tick={{ fill: axis, fontSize: 11 }}
            axisLine={false}
            tickLine={false}
            tickFormatter={(v) => (format ? format(Number(v)) : `${v}${unit ?? ""}`)}
          />
          <Tooltip
            contentStyle={{
              background: "hsl(var(--card))",
              border: "1px solid hsl(var(--border))",
              borderRadius: 8,
              fontSize: 12,
              color: "hsl(var(--card-foreground))",
            }}
            labelStyle={{ color: "hsl(var(--muted-foreground))" }}
            formatter={(v: number) => [
              format ? format(v) : `${v}${unit ?? ""}`,
              title,
            ]}
          />
          <Area
            type="monotone"
            dataKey={dataKey}
            stroke={stroke}
            strokeWidth={2}
            fill={`url(#${gradId})`}
            dot={false}
            isAnimationActive={false}
          />
        </AreaChart>
      </ResponsiveContainer>
    </Card>
  );
}
