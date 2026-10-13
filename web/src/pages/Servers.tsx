import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { Card, Badge, PageHeader, EmptyState, Skeleton } from "@/components/ui";
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
  type LucideIcon,
} from "lucide-react";

type Process = { pid: number; name: string; cpu: number; mem_rss: number; mem_perc: number };
type Container = { id: string; name: string; image: string; state: string };
type Current = {
  cpu_percent: number;
  mem_total: number;
  mem_used: number;
  disk_total: number;
  disk_used: number;
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

export default function Servers() {
  const [cur, setCur] = useState<Current | null>(null);
  const [samples, setSamples] = useState<Sample[]>([]);
  const [loading, setLoading] = useState(true);

  async function loadCurrent() {
    setCur(await api.get<Current>("/api/servers/current"));
  }
  async function loadSamples() {
    setSamples(await api.get<Sample[]>("/api/servers/samples"));
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

  const chartData = samples.map((s) => ({
    t: new Date(s.taken_at).toLocaleTimeString(),
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
                    </tr>
                  </thead>
                  <tbody>
                    {cur.top_procs.map((p) => (
                      <tr
                        key={p.pid}
                        className="border-b border-border/60 last:border-0"
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
                      </tr>
                    </thead>
                    <tbody>
                      {cur.docker.containers.map((c) => (
                        <tr
                          key={c.id}
                          className="border-b border-border/60 last:border-0"
                        >
                          <td className="py-1.5 font-medium">{c.name}</td>
                          <td className="py-1.5 font-mono text-xs text-muted-foreground">
                            {c.image}
                          </td>
                          <td className="py-1.5">
                            <Badge variant={stateVariant(c.state)}>{c.state}</Badge>
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
    </div>
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
          <XAxis dataKey="t" hide />
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
