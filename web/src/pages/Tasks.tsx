import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import { Button, Card, Input, Badge } from "@/components/ui";
import { cn } from "@/lib/api";

type Task = {
  id: number;
  title: string;
  status: "todo" | "doing" | "done";
  position: number;
  due_at: string | null;
  project_id: number | null;
};
type Project = { id: number; name: string };

const columns: { key: Task["status"]; label: string }[] = [
  { key: "todo", label: "Todo" },
  { key: "doing", label: "Doing" },
  { key: "done", label: "Done" },
];

function dueClass(due: string | null): string {
  if (!due) return "";
  const d = new Date(due).getTime();
  const now = Date.now();
  const day = 24 * 60 * 60 * 1000;
  if (d < now) return "text-destructive font-medium";
  if (d - now < 2 * day) return "text-amber-600 font-medium";
  return "text-muted-foreground";
}

type DropTarget = { status: Task["status"]; index: number };

export default function Tasks() {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [title, setTitle] = useState("");
  const [projectId, setProjectId] = useState<number | "">("");
  const [newProject, setNewProject] = useState("");

  const dragId = useRef<number | null>(null);
  const [dragging, setDragging] = useState<number | null>(null);
  const [dropTarget, setDropTarget] = useState<DropTarget | null>(null);

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

  function column(status: Task["status"]) {
    return tasks.filter((t) => t.status === status);
  }

  // --- drag and drop ---

  function onDragStart(id: number) {
    dragId.current = id;
    setDragging(id);
  }

  function onDragEnd() {
    dragId.current = null;
    setDragging(null);
    setDropTarget(null);
  }

  // Compute the insertion index within a column based on pointer position
  // relative to the card being hovered (top half = before, bottom half = after).
  function onCardDragOver(e: React.DragEvent, status: Task["status"], cardIndex: number) {
    e.preventDefault();
    const rect = e.currentTarget.getBoundingClientRect();
    const after = e.clientY - rect.top > rect.height / 2;
    setDropTarget({ status, index: after ? cardIndex + 1 : cardIndex });
  }

  function onColumnDragOver(e: React.DragEvent, status: Task["status"]) {
    e.preventDefault();
    // Default to the end of the column when not hovering a specific card.
    setDropTarget((prev) =>
      prev && prev.status === status ? prev : { status, index: column(status).length },
    );
  }

  async function onDrop(status: Task["status"]) {
    const id = dragId.current;
    const target = dropTarget;
    onDragEnd();
    if (id == null) return;

    const col = column(status).filter((t) => t.id !== id);
    const index = target ? Math.min(target.index, col.length) : col.length;
    const ids = [...col.map((t) => t.id)];
    ids.splice(index, 0, id);

    // Optimistic: reassign status/position locally so the board doesn't flash.
    setTasks((prev) =>
      prev.map((t) => (t.id === id ? { ...t, status } : t)),
    );

    const updated = await api.post<Task[]>("/api/tasks/reorder", { status, ids });
    setTasks(updated);
  }

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">Tasks</h1>
        <p className="text-sm text-muted-foreground">
          A three-column board. Drag cards between columns or reorder within one.
        </p>
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
        {columns.map((col) => {
          const items = column(col.key);
          const showEndMarker =
            dropTarget?.status === col.key && dropTarget.index >= items.length;
          return (
            <div
              key={col.key}
              onDragOver={(e) => onColumnDragOver(e, col.key)}
              onDrop={() => onDrop(col.key)}
              className={cn(
                "flex min-h-[8rem] flex-col gap-3 rounded-lg border border-dashed border-transparent p-1 transition",
                dropTarget?.status === col.key && "border-primary/40 bg-muted/40",
              )}
            >
              <h2 className="px-1 text-sm font-semibold uppercase text-muted-foreground">
                {col.label}
              </h2>
              {items.map((t, i) => (
                <div key={t.id}>
                  {dropTarget?.status === col.key && dropTarget.index === i && (
                    <div className="mb-3 h-0.5 rounded bg-primary" />
                  )}
                  <Card
                    draggable
                    onDragStart={() => onDragStart(t.id)}
                    onDragEnd={onDragEnd}
                    onDragOver={(e) => onCardDragOver(e, col.key, i)}
                    className={cn(
                      "cursor-grab space-y-2 p-3 active:cursor-grabbing",
                      dragging === t.id && "opacity-50",
                    )}
                  >
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
                      <input
                        type="date"
                        className="h-7 rounded border border-border bg-background px-1 text-xs"
                        value={t.due_at ? t.due_at.slice(0, 10) : ""}
                        onChange={(e) =>
                          patch(t.id, {
                            due_at: e.target.value
                              ? new Date(e.target.value).toISOString()
                              : null,
                          } as Partial<Task>)
                        }
                      />
                    </div>
                    {t.due_at && (
                      <div className={cn("text-xs", dueClass(t.due_at))}>
                        Due {new Date(t.due_at).toLocaleDateString()}
                      </div>
                    )}
                  </Card>
                </div>
              ))}
              {showEndMarker && <div className="h-0.5 rounded bg-primary" />}
            </div>
          );
        })}
      </div>
    </div>
  );
}
