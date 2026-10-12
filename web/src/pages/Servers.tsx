import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { Card, Badge } from "@/components/ui";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  CartesianGrid,
} from "recharts";

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

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <Card className="p-4">
      <div className="text-xs uppercase text-muted-foreground">{label}</div>
      <div className="mt-1 text-xl font-semibold">{value}</div>
    </Card>
  );
}

export default function Servers() {
  const [cur, setCur] = useState<Current | null>(null);
  const [samples, setSamples] = useState<Sample[]>([]);

  async function loadCurrent() {
    setCur(await api.get<Current>("/api/servers/current"));
  }
  async function loadSamples() {
    setSamples(await api.get<Sample[]>("/api/servers/samples"));
  }

  useEffect(() => {
    loadCurrent();
    loadSamples();
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
      <div>
        <h1 className="text-2xl font-semibold">Servers</h1>
        <p className="text-sm text-muted-foreground">Live vitals for this Host.</p>
      </div>

      {cur && (
        <>
          <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-6">
            <Stat label="CPU" value={`${cur.cpu_percent.toFixed(1)}%`} />
            <Stat label="Memory" value={`${bytes(cur.mem_used)} / ${bytes(cur.mem_total)}`} />
            <Stat label="Disk" value={`${bytes(cur.disk_used)} / ${bytes(cur.disk_total)}`} />
            <Stat label="Network" value={`↓${bytes(cur.net_rx)} ↑${bytes(cur.net_tx)}`} />
            <Stat label="Load" value={`${cur.load1.toFixed(2)} ${cur.load5.toFixed(2)} ${cur.load15.toFixed(2)}`} />
            <Stat label="Uptime" value={uptime(cur.uptime_secs)} />
          </div>

          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Chart title="CPU %" data={chartData} dataKey="cpu" />
            <Chart title="Memory %" data={chartData} dataKey="memPct" />
            <Chart title="Disk %" data={chartData} dataKey="diskPct" />
            <Chart title="Network (bytes)" data={chartData} dataKey="net" />
          </div>

          <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
            <Card className="p-4">
              <h2 className="mb-2 font-semibold">Top processes</h2>
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-xs text-muted-foreground">
                    <th className="pb-1">PID</th>
                    <th>Name</th>
                    <th>CPU%</th>
                    <th>Mem</th>
                  </tr>
                </thead>
                <tbody>
                  {cur.top_procs.map((p) => (
                    <tr key={p.pid} className="border-t border-border">
                      <td className="py-1">{p.pid}</td>
                      <td>{p.name}</td>
                      <td>{p.cpu.toFixed(1)}</td>
                      <td>{bytes(p.mem_rss)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </Card>

            <Card className="p-4">
              <h2 className="mb-2 font-semibold">Docker</h2>
              {!cur.docker.enabled ? (
                <p className="text-sm text-muted-foreground">Not enabled.</p>
              ) : (
                <table className="w-full text-sm">
                  <thead>
                    <tr className="text-left text-xs text-muted-foreground">
                      <th className="pb-1">Name</th>
                      <th>Image</th>
                      <th>State</th>
                    </tr>
                  </thead>
                  <tbody>
                    {cur.docker.containers.map((c) => (
                      <tr key={c.id} className="border-t border-border">
                        <td className="py-1">{c.name}</td>
                        <td>{c.image}</td>
                        <td>
                          <Badge>{c.state}</Badge>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </Card>
          </div>
        </>
      )}
    </div>
  );
}

function Chart({
  title,
  data,
  dataKey,
}: {
  title: string;
  data: Record<string, number | string>[];
  dataKey: string;
}) {
  return (
    <Card className="p-4">
      <h2 className="mb-2 font-semibold">{title}</h2>
      <ResponsiveContainer width="100%" height={180}>
        <LineChart data={data}>
          <CartesianGrid strokeDasharray="3 3" stroke="#eee" />
          <XAxis dataKey="t" hide />
          <YAxis width={40} />
          <Tooltip />
          <Line type="monotone" dataKey={dataKey} stroke="#0f172a" dot={false} />
        </LineChart>
      </ResponsiveContainer>
    </Card>
  );
}
