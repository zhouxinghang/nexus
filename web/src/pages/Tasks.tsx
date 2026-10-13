import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import {
  Button,
  Card,
  Input,
  Badge,
  Select,
  PageHeader,
  EmptyState,
} from "@/components/ui";
import { cn } from "@/lib/api";
import { CheckSquare, Plus, Trash2, CalendarClock } from "lucide-react";

type Task = {
  id: number;
  title: string;
  status: "todo" | "doing" | "done";
  position: number;
  due_at: string | null;
  project_id: number | null;
};
type Project = { id: number; name: string };

const columns: {
  key: Task["status"];
  label: string;
  dot: string;
}[] = [
  { key: "todo", label: "Todo", dot: "bg-muted-foreground" },
  { key: "doing", label: "Doing", dot: "bg-warning" },
  { key: "done", label: "Done", dot: "bg-success" },
];

function dueClass(due: string | null): string {
  if (!due) return "";
  const d = new Date(due).getTime();
  const now = Date.now();
  const day = 24 * 60 * 60 * 1000;
  if (d < now) return "text-destructive";
  if (d - now < 2 * day) return "text-warning";
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
    setTasks((prev) => prev.map((t) => (t.id === id ? { ...t, status } : t)));

    const updated = await api.post<Task[]>("/api/tasks/reorder", { status, ids });
    setTasks(updated);
  }

  return (
    <div className="space-y-6">
      <PageHeader
        icon={CheckSquare}
        title="Tasks"
        description="A three-column board. Drag cards between columns or reorder within one."
      />

      <Card className="flex flex-col gap-3 p-4 lg:flex-row lg:items-center">
        <form onSubmit={create} className="flex flex-1 flex-wrap gap-2">
          <Input
            className="min-w-[12rem] flex-1"
            placeholder="New task title…"
            value={title}
            onChange={(e) => setTitle(e.target.value)}
          />
          <Select
            value={projectId}
            onChange={(e) =>
              setProjectId(e.target.value === "" ? "" : Number(e.target.value))
            }
          >
            <option value="">No project</option>
            {projects.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </Select>
          <Button type="submit" disabled={!title.trim()}>
            <Plus size={15} /> Add
          </Button>
        </form>
        <div className="hidden w-px self-stretch bg-border lg:block" />
        <form onSubmit={addProject} className="flex gap-2">
          <Input
            className="w-40"
            placeholder="New project…"
            value={newProject}
            onChange={(e) => setNewProject(e.target.value)}
          />
          <Button variant="outline" type="submit" disabled={!newProject.trim()}>
            Add project
          </Button>
        </form>
      </Card>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-3">
        {columns.map((col) => {
          const items = column(col.key);
          const showEndMarker =
            dropTarget?.status === col.key && dropTarget.index >= items.length;
          const isTarget = dropTarget?.status === col.key;
          return (
            <div
              key={col.key}
              onDragOver={(e) => onColumnDragOver(e, col.key)}
              onDrop={() => onDrop(col.key)}
              className={cn(
                "flex min-h-[12rem] flex-col gap-3 rounded-lg border p-3 transition-colors",
                isTarget
                  ? "border-primary/40 bg-accent/40"
                  : "border-border bg-muted/30",
              )}
            >
              <div className="flex items-center justify-between px-1">
                <div className="flex items-center gap-2">
                  <span className={cn("h-2 w-2 rounded-full", col.dot)} />
                  <h2 className="text-sm font-semibold">{col.label}</h2>
                </div>
                <Badge>{items.length}</Badge>
              </div>

              {items.length === 0 && !isTarget && (
                <p className="px-1 py-6 text-center text-xs text-muted-foreground">
                  Nothing here.
                </p>
              )}

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
                      "group cursor-grab space-y-2 p-3 transition active:cursor-grabbing hover:border-primary/30",
                      dragging === t.id && "opacity-50 ring-2 ring-primary/40",
                    )}
                  >
                    <div className="flex items-start justify-between gap-2">
                      <span className="text-sm font-medium leading-snug">
                        {t.title}
                      </span>
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        onClick={() => remove(t.id)}
                        aria-label="Delete task"
                        className="shrink-0 text-muted-foreground opacity-0 transition hover:text-destructive group-hover:opacity-100"
                      >
                        <Trash2 size={14} />
                      </Button>
                    </div>
                    <div className="flex flex-wrap items-center gap-2">
                      {projectName(t.project_id) && (
                        <Badge variant="primary">{projectName(t.project_id)}</Badge>
                      )}
                      <label
                        className={cn(
                          "inline-flex items-center gap-1 rounded border border-border bg-background px-1.5 py-0.5 text-xs",
                          dueClass(t.due_at),
                        )}
                      >
                        <CalendarClock size={12} />
                        <input
                          type="date"
                          className="bg-transparent text-xs outline-none"
                          value={t.due_at ? t.due_at.slice(0, 10) : ""}
                          onChange={(e) =>
                            patch(t.id, {
                              due_at: e.target.value
                                ? new Date(e.target.value).toISOString()
                                : null,
                            } as Partial<Task>)
                          }
                        />
                      </label>
                    </div>
                  </Card>
                </div>
              ))}
              {showEndMarker && <div className="h-0.5 rounded bg-primary" />}
            </div>
          );
        })}
      </div>

      {tasks.length === 0 && (
        <EmptyState
          icon={CheckSquare}
          title="No tasks yet"
          description="Add your first task above to start the board."
        />
      )}
    </div>
  );
}
