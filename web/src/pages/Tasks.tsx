import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import {
  Button,
  Card,
  Input,
  Textarea,
  Badge,
  Select,
  Modal,
  PageHeader,
  EmptyState,
} from "@/components/ui";
import { cn } from "@/lib/api";
import {
  CheckSquare,
  Plus,
  Trash2,
  CalendarClock,
  Flag,
  AlignLeft,
  GripVertical,
  Pencil,
} from "lucide-react";

type Priority = "none" | "low" | "medium" | "high" | "urgent";

type Task = {
  id: number;
  title: string;
  description: string;
  status: "todo" | "doing" | "done";
  priority: Priority;
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

// Priority is metadata (a visual flag), not an ordering key — drag-and-drop
// still governs column order. Ranked high→low so the composer and selects read
// naturally.
const priorities: {
  key: Priority;
  label: string;
  dot: string;
  text: string;
  bar: string;
}[] = [
  { key: "urgent", label: "Urgent", dot: "bg-destructive", text: "text-destructive", bar: "bg-destructive" },
  { key: "high", label: "High", dot: "bg-orange-500", text: "text-orange-600 dark:text-orange-400", bar: "bg-orange-500" },
  { key: "medium", label: "Medium", dot: "bg-warning", text: "text-warning", bar: "bg-warning" },
  { key: "low", label: "Low", dot: "bg-sky-500", text: "text-sky-600 dark:text-sky-400", bar: "bg-sky-500" },
  { key: "none", label: "None", dot: "bg-muted-foreground/40", text: "text-muted-foreground", bar: "" },
];

const priorityMeta = (p: Priority) =>
  priorities.find((x) => x.key === p) ?? priorities[priorities.length - 1];

function dueClass(due: string | null): string {
  if (!due) return "";
  const d = new Date(due).getTime();
  const now = Date.now();
  const day = 24 * 60 * 60 * 1000;
  if (d < now) return "text-destructive";
  if (d - now < 2 * day) return "text-warning";
  return "text-muted-foreground";
}

// A short, human label for a due date relative to today.
function dueLabel(due: string | null): string {
  if (!due) return "";
  const d = new Date(due);
  const startOfDay = (t: Date) =>
    new Date(t.getFullYear(), t.getMonth(), t.getDate()).getTime();
  const day = 24 * 60 * 60 * 1000;
  const diff = Math.round((startOfDay(d) - startOfDay(new Date())) / day);
  if (diff === 0) return "Today";
  if (diff === 1) return "Tomorrow";
  if (diff === -1) return "Yesterday";
  if (diff < 0) return `${-diff}d overdue`;
  if (diff < 7) return `In ${diff}d`;
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

type DropTarget = { status: Task["status"]; index: number };

export default function Tasks() {
  const [tasks, setTasks] = useState<Task[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [title, setTitle] = useState("");
  const [composerPriority, setComposerPriority] = useState<Priority>("none");
  const [projectId, setProjectId] = useState<number | "">("");
  const [newProject, setNewProject] = useState("");
  const [projectFilter, setProjectFilter] = useState<number | "all">("all");
  const [editing, setEditing] = useState<Task | null>(null);

  const dragId = useRef<number | null>(null);
  const didDrag = useRef(false);
  const [dragging, setDragging] = useState<number | null>(null);
  const [dropTarget, setDropTarget] = useState<DropTarget | null>(null);

  async function load() {
    const [t, p] = await Promise.all([
      api.get<Task[]>("/api/tasks"),
      api.get<Project[]>("/api/projects"),
    ]);
    setTasks(t);
    setProjects(p);
  }
  useEffect(() => {
    load();
  }, []);

  async function create(e: React.FormEvent) {
    e.preventDefault();
    if (!title.trim()) return;
    await api.post("/api/tasks", {
      title,
      priority: composerPriority,
      project_id: projectId === "" ? null : projectId,
    });
    setTitle("");
    setComposerPriority("none");
    load();
  }

  async function patch(id: number, body: Partial<Task>) {
    const updated = await api.patch<Task>(`/api/tasks/${id}`, body);
    setTasks((prev) => prev.map((t) => (t.id === id ? updated : t)));
    setEditing((e) => (e && e.id === id ? updated : e));
  }

  async function remove(id: number) {
    await api.del(`/api/tasks/${id}`);
    setEditing((e) => (e && e.id === id ? null : e));
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

  function visible(): Task[] {
    if (projectFilter === "all") return tasks;
    return tasks.filter((t) => t.project_id === projectFilter);
  }

  function column(status: Task["status"]) {
    return visible().filter((t) => t.status === status);
  }

  // --- drag and drop ---

  function onDragStart(id: number) {
    dragId.current = id;
    didDrag.current = true;
    setDragging(id);
  }

  function onDragEnd() {
    dragId.current = null;
    setDragging(null);
    setDropTarget(null);
    // Let the click handler that fires right after a drag know to ignore it.
    setTimeout(() => {
      didDrag.current = false;
    }, 0);
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

    // Optimistic: reassign status locally so the board doesn't flash.
    setTasks((prev) => prev.map((t) => (t.id === id ? { ...t, status } : t)));

    const updated = await api.post<Task[]>("/api/tasks/reorder", { status, ids });
    setTasks(updated);
  }

  function openEditor(t: Task) {
    if (didDrag.current) return;
    setEditing(t);
  }

  return (
    <div className="space-y-6">
      <PageHeader
        icon={CheckSquare}
        title="Tasks"
        description="A three-column board. Drag cards between columns or reorder within one; click a card to edit its details."
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
            value={composerPriority}
            onChange={(e) => setComposerPriority(e.target.value as Priority)}
            title="Priority"
          >
            {priorities.map((p) => (
              <option key={p.key} value={p.key}>
                {p.key === "none" ? "No priority" : `${p.label} priority`}
              </option>
            ))}
          </Select>
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

      {projects.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="mr-1 text-xs font-medium text-muted-foreground">
            Project:
          </span>
          <FilterChip
            active={projectFilter === "all"}
            onClick={() => setProjectFilter("all")}
          >
            All
          </FilterChip>
          {projects.map((p) => (
            <FilterChip
              key={p.id}
              active={projectFilter === p.id}
              onClick={() => setProjectFilter(p.id)}
            >
              {p.name}
            </FilterChip>
          ))}
        </div>
      )}

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

              {items.map((t, i) => {
                const pm = priorityMeta(t.priority);
                return (
                  <div key={t.id}>
                    {dropTarget?.status === col.key && dropTarget.index === i && (
                      <div className="mb-3 h-0.5 rounded bg-primary" />
                    )}
                    <Card
                      draggable
                      onDragStart={() => onDragStart(t.id)}
                      onDragEnd={onDragEnd}
                      onDragOver={(e) => onCardDragOver(e, col.key, i)}
                      onClick={() => openEditor(t)}
                      className={cn(
                        "group relative cursor-pointer space-y-2 overflow-hidden p-3 pl-3.5 transition hover:border-primary/30 hover:shadow-pop",
                        dragging === t.id && "opacity-50 ring-2 ring-primary/40",
                      )}
                    >
                      {pm.bar && (
                        <span
                          className={cn(
                            "absolute inset-y-0 left-0 w-1",
                            pm.bar,
                          )}
                          aria-hidden
                        />
                      )}
                      <div className="flex items-start justify-between gap-2">
                        <span
                          className={cn(
                            "text-sm font-medium leading-snug",
                            t.status === "done" &&
                              "text-muted-foreground line-through",
                          )}
                        >
                          {t.title}
                        </span>
                        <div className="flex shrink-0 items-center gap-0.5">
                          <GripVertical
                            size={14}
                            className="cursor-grab text-muted-foreground/40 opacity-0 transition group-hover:opacity-100"
                            aria-hidden
                          />
                          <Button
                            variant="ghost"
                            size="icon-sm"
                            onClick={(e) => {
                              e.stopPropagation();
                              remove(t.id);
                            }}
                            aria-label="Delete task"
                            className="text-muted-foreground opacity-0 transition hover:text-destructive group-hover:opacity-100"
                          >
                            <Trash2 size={14} />
                          </Button>
                        </div>
                      </div>

                      {t.description && (
                        <p className="line-clamp-2 text-xs leading-relaxed text-muted-foreground">
                          {t.description}
                        </p>
                      )}

                      {(t.priority !== "none" ||
                        projectName(t.project_id) ||
                        t.due_at) && (
                        <div className="flex flex-wrap items-center gap-1.5">
                          {t.priority !== "none" && (
                            <span
                              className={cn(
                                "inline-flex items-center gap-1 rounded-full border border-border bg-background px-1.5 py-0.5 text-[11px] font-medium",
                                pm.text,
                              )}
                            >
                              <Flag size={11} className="fill-current" />
                              {pm.label}
                            </span>
                          )}
                          {projectName(t.project_id) && (
                            <Badge variant="primary">
                              {projectName(t.project_id)}
                            </Badge>
                          )}
                          {t.due_at && (
                            <span
                              className={cn(
                                "inline-flex items-center gap-1 rounded-full border border-border bg-background px-1.5 py-0.5 text-[11px] font-medium",
                                dueClass(t.due_at),
                              )}
                            >
                              <CalendarClock size={11} />
                              {dueLabel(t.due_at)}
                            </span>
                          )}
                        </div>
                      )}
                    </Card>
                  </div>
                );
              })}
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

      {editing && (
        <TaskEditor
          key={editing.id}
          task={editing}
          projects={projects}
          onClose={() => setEditing(null)}
          onPatch={patch}
          onDelete={remove}
        />
      )}
    </div>
  );
}

function FilterChip({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "rounded-full border px-2.5 py-0.5 text-xs font-medium transition-colors",
        active
          ? "border-transparent bg-primary text-primary-foreground"
          : "border-border bg-background text-muted-foreground hover:bg-muted hover:text-foreground",
      )}
    >
      {children}
    </button>
  );
}

function TaskEditor({
  task,
  projects,
  onClose,
  onPatch,
  onDelete,
}: {
  task: Task;
  projects: Project[];
  onClose: () => void;
  onPatch: (id: number, body: Partial<Task>) => Promise<void>;
  onDelete: (id: number) => void;
}) {
  const [title, setTitle] = useState(task.title);
  const [description, setDescription] = useState(task.description);
  const [status, setStatus] = useState<Task["status"]>(task.status);
  const [priority, setPriority] = useState<Priority>(task.priority);
  const [projectId, setProjectId] = useState<number | "">(
    task.project_id ?? "",
  );
  const [due, setDue] = useState(task.due_at ? task.due_at.slice(0, 10) : "");
  const [saving, setSaving] = useState(false);

  const dirty =
    title !== task.title ||
    description !== task.description ||
    status !== task.status ||
    priority !== task.priority ||
    (projectId === "" ? null : projectId) !== task.project_id ||
    (due || null) !== (task.due_at ? task.due_at.slice(0, 10) : null);

  async function save() {
    if (!title.trim() || saving) return;
    setSaving(true);
    try {
      await onPatch(task.id, {
        title: title.trim(),
        description,
        status,
        priority,
        project_id: projectId === "" ? null : projectId,
        due_at: due ? new Date(due).toISOString() : null,
      } as Partial<Task>);
      onClose();
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal
      open
      onClose={onClose}
      icon={Pencil}
      title="Edit task"
      description="Update any field and save your changes."
      footer={
        <>
          <Button
            variant="destructive"
            size="sm"
            className="mr-auto"
            onClick={() => {
              if (confirm("Delete this task?")) onDelete(task.id);
            }}
          >
            <Trash2 size={14} /> Delete
          </Button>
          <Button variant="outline" size="sm" onClick={onClose}>
            Cancel
          </Button>
          <Button
            size="sm"
            loading={saving}
            disabled={!title.trim() || !dirty}
            onClick={save}
          >
            Save changes
          </Button>
        </>
      }
    >
      <div className="space-y-4">
        <div className="space-y-1.5">
          <label className="text-xs font-medium text-muted-foreground">
            Title
          </label>
          <Input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Task title"
            autoFocus
          />
        </div>

        <div className="space-y-1.5">
          <label className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
            <AlignLeft size={13} /> Description
          </label>
          <Textarea
            rows={4}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Add more detail, acceptance criteria, links…"
          />
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <label className="text-xs font-medium text-muted-foreground">
              Status
            </label>
            <Select
              className="w-full"
              value={status}
              onChange={(e) => setStatus(e.target.value as Task["status"])}
            >
              {columns.map((c) => (
                <option key={c.key} value={c.key}>
                  {c.label}
                </option>
              ))}
            </Select>
          </div>

          <div className="space-y-1.5">
            <label className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
              <Flag size={13} /> Priority
            </label>
            <Select
              className="w-full"
              value={priority}
              onChange={(e) => setPriority(e.target.value as Priority)}
            >
              {priorities.map((p) => (
                <option key={p.key} value={p.key}>
                  {p.label}
                </option>
              ))}
            </Select>
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-medium text-muted-foreground">
              Project
            </label>
            <Select
              className="w-full"
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
          </div>

          <div className="space-y-1.5">
            <label className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
              <CalendarClock size={13} /> Due date
            </label>
            <div className="flex gap-2">
              <Input
                type="date"
                className="w-full"
                value={due}
                onChange={(e) => setDue(e.target.value)}
              />
              {due && (
                <Button
                  variant="outline"
                  size="icon"
                  aria-label="Clear due date"
                  onClick={() => setDue("")}
                >
                  <Trash2 size={14} />
                </Button>
              )}
            </div>
          </div>
        </div>
      </div>
    </Modal>
  );
}
