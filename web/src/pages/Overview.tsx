import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, cn } from "@/lib/api";
import { useToast } from "@/lib/toast";
import { Badge, Card, PageHeader, Skeleton } from "@/components/ui";
import {
  LayoutDashboard,
  Server,
  CheckSquare,
  BookText,
  Compass,
  FileText,
  ArrowRight,
  AlertTriangle,
  CalendarClock,
  type LucideIcon,
} from "lucide-react";

type Overview = {
  host: {
    available: boolean;
    cpu_percent: number;
    mem_total: number;
    mem_used: number;
    mem_percent: number;
    disk_total: number;
    disk_used: number;
    disk_percent: number;
    load1: number;
    load5: number;
    load15: number;
    uptime_secs: number;
    docker_enabled: boolean;
    containers: number;
    flags: { cpu: boolean; mem: boolean; disk: boolean };
  };
  tasks: { todo: number; doing: number; done: number; overdue: number; due_today: number };
  journal: { id: number; body: string; created_at: string }[];
  bookmarks: {
    up: number;
    down: number;
    unknown: number;
    down_list: { id: number; name: string; url: string; last_checked_at: string | null }[];
  };
  files: { total: number; missing: number };
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

// Overview is the workbench's landing summary: one round trip to
// /api/overview, one panel per tool, each linking into it. Threshold flags are
// purely visual.
export default function OverviewPage() {
  const [data, setData] = useState<Overview | null>(null);
  const toast = useToast();

  useEffect(() => {
    let alive = true;
    async function load() {
      try {
        const o = await api.get<Overview>("/api/overview");
        if (alive) setData(o);
      } catch (err) {
        if (alive) toast.error((err as Error).message);
      }
    }
    load();
    // Keep vitals reasonably fresh without hammering the Host.
    const t = setInterval(load, 15000);
    return () => {
      alive = false;
      clearInterval(t);
    };
  }, []);

  return (
    <div className="space-y-6">
      <PageHeader
        icon={LayoutDashboard}
        title="Overview"
        description="The state of your workbench at a glance. Each panel opens its tool."
      />

      {!data ? (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {Array.from({ length: 5 }).map((_, i) => (
            <Skeleton key={i} className="h-44" />
          ))}
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          <HostPanel host={data.host} />
          <TasksPanel tasks={data.tasks} />
          <BookmarksPanel bookmarks={data.bookmarks} />
          <FilesPanel files={data.files} />
          <JournalPanel journal={data.journal} />
        </div>
      )}
    </div>
  );
}

function Panel({
  to,
  icon: Icon,
  title,
  className,
  children,
}: {
  to: string;
  icon: LucideIcon;
  title: string;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <Link
      to={to}
      className={cn(
        "group block rounded-lg focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        className,
      )}
    >
      <Card interactive className="flex h-full flex-col p-4">
        <div className="mb-3 flex items-center gap-2">
          <Icon size={16} className="text-muted-foreground" />
          <h2 className="font-semibold">{title}</h2>
          <ArrowRight
            size={15}
            className="ml-auto text-muted-foreground/50 transition group-hover:translate-x-0.5 group-hover:text-foreground"
          />
        </div>
        <div className="flex-1">{children}</div>
      </Card>
    </Link>
  );
}

function Metric({
  label,
  value,
  hint,
  tone,
}: {
  label: string;
  value: React.ReactNode;
  hint?: React.ReactNode;
  tone?: "danger" | "warning" | "success";
}) {
  return (
    <div className="min-w-0">
      <p className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
        {label}
      </p>
      <p
        className={cn(
          "text-lg font-semibold tabular-nums",
          tone === "danger" && "text-destructive",
          tone === "warning" && "text-warning",
          tone === "success" && "text-success",
        )}
      >
        {value}
      </p>
      {hint && <p className="truncate text-xs text-muted-foreground">{hint}</p>}
    </div>
  );
}

function UsageBar({ pct, flagged }: { pct: number; flagged: boolean }) {
  return (
    <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-muted">
      <div
        className={cn("h-full rounded-full", flagged ? "bg-destructive" : "bg-primary")}
        style={{ width: `${Math.min(100, Math.max(0, pct))}%` }}
      />
    </div>
  );
}

function HostPanel({ host }: { host: Overview["host"] }) {
  if (!host.available) {
    return (
      <Panel to="/servers" icon={Server} title="Host">
        <p className="text-sm text-muted-foreground">Could not read Host vitals right now.</p>
      </Panel>
    );
  }
  const anyFlag = host.flags.cpu || host.flags.mem || host.flags.disk;
  return (
    <Panel to="/servers" icon={Server} title="Host">
      <div className="space-y-3">
        <div className="grid grid-cols-3 gap-3">
          <div>
            <Metric
              label="CPU"
              value={`${host.cpu_percent.toFixed(0)}%`}
              tone={host.flags.cpu ? "danger" : undefined}
            />
            <UsageBar pct={host.cpu_percent} flagged={host.flags.cpu} />
          </div>
          <div>
            <Metric
              label="Memory"
              value={`${host.mem_percent.toFixed(0)}%`}
              tone={host.flags.mem ? "danger" : undefined}
            />
            <UsageBar pct={host.mem_percent} flagged={host.flags.mem} />
          </div>
          <div>
            <Metric
              label="Disk"
              value={`${host.disk_percent.toFixed(0)}%`}
              tone={host.flags.disk ? "danger" : undefined}
            />
            <UsageBar pct={host.disk_percent} flagged={host.flags.disk} />
          </div>
        </div>
        <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
          <span>
            Mem {bytes(host.mem_used)} / {bytes(host.mem_total)}
          </span>
          <span>
            Disk {bytes(host.disk_used)} / {bytes(host.disk_total)}
          </span>
          <span className="tabular-nums">
            Load {host.load1.toFixed(2)} {host.load5.toFixed(2)} {host.load15.toFixed(2)}
          </span>
          <span>Up {uptime(host.uptime_secs)}</span>
          {host.docker_enabled && <span>{host.containers} containers</span>}
        </div>
        {anyFlag && (
          <p className="flex items-center gap-1.5 text-xs font-medium text-destructive">
            <AlertTriangle size={13} /> Usage at or above 90%
          </p>
        )}
      </div>
    </Panel>
  );
}

function TasksPanel({ tasks }: { tasks: Overview["tasks"] }) {
  const open = tasks.todo + tasks.doing;
  return (
    <Panel to="/tasks" icon={CheckSquare} title="Tasks">
      <div className="space-y-3">
        <div className="grid grid-cols-2 gap-3">
          <Metric
            label="Overdue"
            value={tasks.overdue}
            tone={tasks.overdue > 0 ? "danger" : undefined}
          />
          <Metric
            label="Due today"
            value={tasks.due_today}
            tone={tasks.due_today > 0 ? "warning" : undefined}
          />
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          <Badge>Todo {tasks.todo}</Badge>
          <Badge variant="warning">Doing {tasks.doing}</Badge>
          <Badge variant="success">Done {tasks.done}</Badge>
        </div>
        <p className="text-xs text-muted-foreground">
          {open === 0 ? "Nothing open on the board." : `${open} open on the board.`}
        </p>
      </div>
    </Panel>
  );
}

function BookmarksPanel({ bookmarks }: { bookmarks: Overview["bookmarks"] }) {
  return (
    <Panel to="/bookmarks" icon={Compass} title="Bookmarks">
      <div className="space-y-3">
        <div className="grid grid-cols-3 gap-3">
          <Metric label="Up" value={bookmarks.up} tone={bookmarks.up > 0 ? "success" : undefined} />
          <Metric
            label="Down"
            value={bookmarks.down}
            tone={bookmarks.down > 0 ? "danger" : undefined}
          />
          <Metric label="Unchecked" value={bookmarks.unknown} />
        </div>
        {bookmarks.down_list.length === 0 ? (
          <p className="text-xs text-muted-foreground">All checked sites are reachable.</p>
        ) : (
          <ul className="space-y-1">
            {bookmarks.down_list.slice(0, 4).map((b) => (
              <li key={b.id} className="flex items-center gap-2 text-sm">
                <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-destructive" />
                <span className="truncate font-medium">{b.name}</span>
                <span className="truncate text-xs text-muted-foreground">
                  {hostOf(b.url)}
                </span>
              </li>
            ))}
            {bookmarks.down_list.length > 4 && (
              <li className="text-xs text-muted-foreground">
                +{bookmarks.down_list.length - 4} more down
              </li>
            )}
          </ul>
        )}
      </div>
    </Panel>
  );
}

function hostOf(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}

function FilesPanel({ files }: { files: Overview["files"] }) {
  return (
    <Panel to="/files" icon={FileText} title="Files">
      <div className="space-y-3">
        <div className="grid grid-cols-2 gap-3">
          <Metric label="Resources" value={files.total.toLocaleString()} />
          <Metric
            label="Missing"
            value={files.missing.toLocaleString()}
            tone={files.missing > 0 ? "warning" : undefined}
          />
        </div>
        <p className="text-xs text-muted-foreground">
          {files.total === 0
            ? "No Resources indexed yet. Add a Managed Root to start."
            : files.missing > 0
              ? "Some indexed files are gone from disk. Review and clear them in Files."
              : "Every indexed file is present on disk."}
        </p>
      </div>
    </Panel>
  );
}

function JournalPanel({ journal }: { journal: Overview["journal"] }) {
  return (
    <Panel to="/journal" icon={BookText} title="Latest journal" className="md:col-span-2 xl:col-span-1">
      {journal.length === 0 ? (
        <p className="text-sm text-muted-foreground">No entries yet.</p>
      ) : (
        <ul className="space-y-2.5">
          {journal.map((e) => (
            <li key={e.id} className="min-w-0">
              <p className="flex items-center gap-1 text-[11px] text-muted-foreground">
                <CalendarClock size={11} />
                {new Date(e.created_at).toLocaleString(undefined, {
                  month: "short",
                  day: "numeric",
                  hour: "numeric",
                  minute: "2-digit",
                })}
              </p>
              <p className="line-clamp-2 text-sm leading-snug">{e.body}</p>
            </li>
          ))}
        </ul>
      )}
    </Panel>
  );
}
