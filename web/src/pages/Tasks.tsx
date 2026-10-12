import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { Button, Card, Input, Badge } from "@/components/ui";
import { cn } from "@/lib/api";

type Task = {
  id: number;
  title: string;
  status: "todo" | "doing" | "done";
  priority: "none" | "low" | "medium" | "high";
  due_at: string | null;
  project_id: number | null;
};
type Project = { id: number; name: string };

const columns: { key: Task["status"]; label: string }[] = [
  { key: "todo", label: "Todo" },
  { key: "doing", label: "Doing" },
  { key: "done", label: "Done" },
];

const priorities: Task["priority"][] = ["none", "low", "medium", "high"];

function dueClass(due: string | null): string {
  if (!due) return "";
  const d = new Date(due).getTime();
  const now = Date.now();
  const day = 24 * 60 * 60 * 1000;
  if (d < now) return "text-destructive font-medium";
  if (d - now < 2 * day) return "text-amber-600 font-medium";
  return "text-muted-foreground";
}

export default function Tasks() {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [title, setTitle] = useState("");
  const [projectId, setProjectId] = useState<number | "">("");
  const [newProject, setNewProject] = useState("");

  async function load() {
    setTasks(await api.get<Task[]>("/api/tasks"));
    setProjects(await api.get<Project[]>("/api/projects"));
  }
  useEffect(() => {
    load();
  }, []);

  async function create(e: React.FormEvent) {
    e.preventDefault();
    if (!title.trim()) return;
    await api.post("/api/tasks", {
      title,
      project_id: projectId === "" ? null : projectId,
    });
    setTitle("");
    load();
  }

  async function patch(id: number, body: Partial<Task>) {
    await api.patch(`/api/tasks/${id}`, body);
    load();
  }

  async function remove(id: number) {
    await api.del(`/api/tasks/${id}`);
    load();
  }

  async function addProject(e: React.FormEvent) {
    e.preventDefault();
    if (!newProject.trim()) return;
    await api.post("/api/projects", { name: newProject });
    setNewProject("");
    load();
  }

  function projectName(id: number | null) {
    if (id == null) return null;
    return projects.find((p) => p.id === id)?.name ?? null;
  }

  function move(t: Task, dir: -1 | 1) {
    const idx = columns.findIndex((c) => c.key === t.status);
    const next = columns[idx + dir];
    if (next) patch(t.id, { status: next.key });
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">Tasks</h1>
        <p className="text-sm text-muted-foreground">A three-column board.</p>
      </div>

      <div className="flex flex-wrap gap-2">
        <form onSubmit={create} className="flex flex-1 gap-2">
          <Input
            placeholder="New task title…"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
          />
          <select
            className="h-9 rounded-md border border-border bg-background px-2 text-sm"
            value={projectId}
            onChange={(e) => setProjectId(e.target.value === "" ? "" : Number(e.target.value))}
          >
            <option value="">No project</option>
            {projects.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
          <Button type="submit">Add</Button>
        </form>
        <form onSubmit={addProject} className="flex gap-2">
          <Input
            placeholder="New project…"
            value={newProject}
            onChange={(e) => setNewProject(e.target.value)}
          />
          <Button variant="outline" type="submit">
            Add project
          </Button>
        </form>
      </div>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        {columns.map((col) => (
          <div key={col.key} className="space-y-3">
            <h2 className="px-1 text-sm font-semibold uppercase text-muted-foreground">
              {col.label}
            </h2>
            {tasks
              .filter((t) => t.status === col.key)
              .map((t) => (
                <Card key={t.id} className="space-y-2 p-3">
                  <div className="flex items-start justify-between gap-2">
                    <span className="text-sm font-medium">{t.title}</span>
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => remove(t.id)}
                      className="text-destructive"
                    >
                      ✕
                    </Button>
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    {projectName(t.project_id) && (
                      <Badge>{projectName(t.project_id)}</Badge>
                    )}
                    <select
                      className="h-7 rounded border border-border bg-background px-1 text-xs"
                      value={t.priority}
                      onChange={(e) => patch(t.id, { priority: e.target.value as Task["priority"] })}
                    >
                      {priorities.map((p) => (
                        <option key={p} value={p}>
                          {p}
                        </option>
                      ))}
                    </select>
                    <input
                      type="date"
                      className="h-7 rounded border border-border bg-background px-1 text-xs"
                      value={t.due_at ? t.due_at.slice(0, 10) : ""}
                      onChange={(e) =>
                        patch(t.id, {
                          due_at: e.target.value ? new Date(e.target.value).toISOString() : null,
                        } as Partial<Task>)
                      }
                    />
                  </div>
                  {t.due_at && (
                    <div className={cn("text-xs", dueClass(t.due_at))}>
                      Due {new Date(t.due_at).toLocaleDateString()}
                    </div>
                  )}
                  <div className="flex justify-between pt-1">
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={col.key === "todo"}
                      onClick={() => move(t, -1)}
                    >
                      ←
                    </Button>
                    <Button
                      variant="ghost"
                      size="sm"
                      disabled={col.key === "done"}
                      onClick={() => move(t, 1)}
                    >
                      →
                    </Button>
                  </div>
                </Card>
              ))}
          </div>
        ))}
      </div>
    </div>
  );
}
