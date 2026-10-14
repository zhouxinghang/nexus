import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import { useToast } from "@/lib/toast";
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
  Spinner,
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
  Columns3,
  Type,
  Archive,
  ArchiveRestore,
  Tag,
  ListChecks,
  Check,
  X,
} from "lucide-react";

type Priority = "none" | "low" | "medium" | "high" | "urgent";

type LabelColor =
  | "gray"
  | "red"
  | "orange"
  | "amber"
  | "green"
  | "teal"
  | "blue"
  | "violet"
  | "pink";

type Label = { id: number; name: string; color: LabelColor };

type Subtask = {
  id: number;
  task_id: number;
  title: string;
  done: boolean;
  position: number;
};

type Task = {
  id: number;
  title: string;
  description: string;
  status: "todo" | "doing" | "done";
  priority: Priority;
  position: number;
  due_at: string | null;
  archived_at: string | null;
  labels: Label[];
  subtasks: Subtask[];
  subtask_progress: { done: number; total: number };
};

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

// The fixed label palette; keys match the server's allowed color tokens.
// Labels are a filter/visual only, never an ordering key.
const labelColors: { key: LabelColor; chip: string; dot: string }[] = [
  { key: "gray", chip: "bg-muted text-muted-foreground", dot: "bg-muted-foreground" },
  { key: "red", chip: "bg-red-500/15 text-red-700 dark:text-red-400", dot: "bg-red-500" },
  { key: "orange", chip: "bg-orange-500/15 text-orange-700 dark:text-orange-400", dot: "bg-orange-500" },
  { key: "amber", chip: "bg-amber-500/15 text-amber-700 dark:text-amber-400", dot: "bg-amber-500" },
  { key: "green", chip: "bg-emerald-500/15 text-emerald-700 dark:text-emerald-400", dot: "bg-emerald-500" },
  { key: "teal", chip: "bg-teal-500/15 text-teal-700 dark:text-teal-400", dot: "bg-teal-500" },
  { key: "blue", chip: "bg-blue-500/15 text-blue-700 dark:text-blue-400", dot: "bg-blue-500" },
  { key: "violet", chip: "bg-violet-500/15 text-violet-700 dark:text-violet-400", dot: "bg-violet-500" },
  { key: "pink", chip: "bg-pink-500/15 text-pink-700 dark:text-pink-400", dot: "bg-pink-500" },
];

const labelStyle = (c: string) =>
  labelColors.find((x) => x.key === c) ?? labelColors[0];

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
  const [labels, setLabels] = useState<Label[]>([]);
  const [title, setTitle] = useState("");
  const [composerPriority, setComposerPriority] = useState<Priority>("none");
  const [labelFilter, setLabelFilter] = useState<number | "all">("all");
  const [editing, setEditing] = useState<Task | null>(null);
  const [labelsOpen, setLabelsOpen] = useState(false);
  const [archivedOpen, setArchivedOpen] = useState(false);
  const toast = useToast();

  const dragId = useRef<number | null>(null);
  const didDrag = useRef(false);
  const [dragging, setDragging] = useState<number | null>(null);
  const [dropTarget, setDropTarget] = useState<DropTarget | null>(null);

  async function load() {
    try {
      const [t, l] = await Promise.all([
        api.get<Task[]>("/api/tasks"),
        api.get<Label[]>("/api/labels"),
      ]);
      setTasks(t);
      setLabels(l);
      // Drop a filter whose label no longer exists.
      setLabelFilter((f) => (f !== "all" && !l.some((x) => x.id === f) ? "all" : f));
    } catch (err) {
      toast.error((err as Error).message);
    }
  }
  useEffect(() => {
    load();
  }, []);

  async function create(e: React.FormEvent) {
    e.preventDefault();
    if (!title.trim()) return;
    try {
      await api.post("/api/tasks", {
        title,
        priority: composerPriority,
      });
      setTitle("");
      setComposerPriority("none");
      load();
      toast.success("Task created");
    } catch (err) {
      toast.error((err as Error).message);
    }
  }

  // syncTask swaps a server-refreshed task into the board and the open editor.
  function syncTask(updated: Task) {
    setTasks((prev) => prev.map((t) => (t.id === updated.id ? updated : t)));
    setEditing((e) => (e && e.id === updated.id ? updated : e));
  }

  async function patch(id: number, body: Partial<Task>) {
    try {
      syncTask(await api.patch<Task>(`/api/tasks/${id}`, body));
    } catch (err) {
      toast.error((err as Error).message);
    }
  }

  async function remove(id: number) {
    try {
      await api.del(`/api/tasks/${id}`);
      setEditing((e) => (e && e.id === id ? null : e));
      load();
      toast.success("Task deleted");
    } catch (err) {
      toast.error((err as Error).message);
    }
  }

  async function archive(id: number) {
    try {
      await api.post(`/api/tasks/${id}/archive`);
      setTasks((prev) => prev.filter((t) => t.id !== id));
      setEditing((e) => (e && e.id === id ? null : e));
      toast.success("Task archived", {
        description: "Find it under Archived to restore it.",
      });
    } catch (err) {
      toast.error((err as Error).message);
    }
  }

  async function archiveDone() {
    const n = tasks.filter((t) => t.status === "done").length;
    if (n === 0) return;
    try {
      setTasks(await api.post<Task[]>("/api/tasks/archive-done"));
      toast.success(`Archived ${n} done ${n === 1 ? "task" : "tasks"}`);
    } catch (err) {
      toast.error((err as Error).message);
    }
  }

  // The label filter runs on the client over the full board (every task
  // carries its labels) so drag-reordering a filtered column can still be
  // written back against the column's complete order.
  function visible(): Task[] {
    if (labelFilter === "all") return tasks;
    return tasks.filter((t) => t.labels.some((l) => l.id === labelFilter));
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

    // The drop index is relative to the visible (possibly filtered) column;
    // translate it to the full column by inserting before the visible card
    // that was under the pointer, or at the end.
    const shown = column(status).filter((t) => t.id !== id);
    const index = target ? Math.min(target.index, shown.length) : shown.length;
    const ids = tasks
      .filter((t) => t.status === status && t.id !== id)
      .map((t) => t.id);
    const anchor = shown[index];
    ids.splice(anchor ? ids.indexOf(anchor.id) : ids.length, 0, id);

    // Optimistic: reassign status locally so the board doesn't flash.
    setTasks((prev) => prev.map((t) => (t.id === id ? { ...t, status } : t)));

    try {
      const updated = await api.post<Task[]>("/api/tasks/reorder", { status, ids });
      setTasks(updated);
    } catch (err) {
      // Roll back to server truth if the reorder failed.
      toast.error((err as Error).message);
      load();
    }
  }

  function openEditor(t: Task) {
    if (didDrag.current) return;
    setEditing(t);
  }

  const doneCount = tasks.filter((t) => t.status === "done").length;

  return (
    <div className="space-y-6">
      <PageHeader
        icon={CheckSquare}
        title="Tasks"
        description="A three-column board. Drag cards between columns or reorder within one; click a card to edit its details, labels, and subtasks."
        actions={
          <>
            <Button variant="outline" size="sm" onClick={() => setLabelsOpen(true)}>
              <Tag size={14} /> Labels
            </Button>
            <Button variant="outline" size="sm" onClick={() => setArchivedOpen(true)}>
              <Archive size={14} /> Archived
            </Button>
          </>
        }
      />

      <Card className="p-4">
        <form onSubmit={create} className="flex flex-wrap gap-2">
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
          <Button type="submit" disabled={!title.trim()}>
            <Plus size={15} /> Add
          </Button>
        </form>
      </Card>

      {labels.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="mr-1 text-xs font-medium text-muted-foreground">
            Label:
          </span>
          <FilterChip
            active={labelFilter === "all"}
            onClick={() => setLabelFilter("all")}
          >
            All
          </FilterChip>
          {labels.map((l) => (
            <FilterChip
              key={l.id}
              active={labelFilter === l.id}
              onClick={() => setLabelFilter(l.id)}
            >
              <span
                className={cn("h-1.5 w-1.5 rounded-full", labelStyle(l.color).dot)}
                aria-hidden
              />
              {l.name}
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
                <div className="flex items-center gap-1">
                  {col.key === "done" && doneCount > 0 && (
                    <Button
                      variant="ghost"
                      size="xs"
                      onClick={archiveDone}
                      title="Archive every task in Done (reversible)"
                      className="text-muted-foreground"
                    >
                      <Archive size={13} /> Archive all
                    </Button>
                  )}
                  <Badge>{items.length}</Badge>
                </div>
              </div>

              {items.length === 0 && !isTarget && (
                <p className="px-1 py-6 text-center text-xs text-muted-foreground">
                  Nothing here.
                </p>
              )}

              {items.map((t, i) => {
                const pm = priorityMeta(t.priority);
                const progress = t.subtask_progress;
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
                        "group relative cursor-pointer space-y-1 overflow-hidden px-3 py-2.5 pl-3.5 transition hover:border-primary/30 hover:shadow-pop",
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
                      <div className="absolute right-1.5 top-1.5 flex items-center gap-0.5">
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
                            archive(t.id);
                          }}
                          aria-label="Archive task"
                          title="Archive"
                          className="h-6 w-6 text-muted-foreground opacity-0 transition hover:text-foreground group-hover:opacity-100"
                        >
                          <Archive size={14} />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          onClick={(e) => {
                            e.stopPropagation();
                            remove(t.id);
                          }}
                          aria-label="Delete task"
                          title="Delete permanently"
                          className="h-6 w-6 text-muted-foreground opacity-0 transition hover:text-destructive group-hover:opacity-100"
                        >
                          <Trash2 size={14} />
                        </Button>
                      </div>
                      <span
                        className={cn(
                          "block pr-[4.5rem] text-sm font-medium leading-snug",
                          t.status === "done" &&
                            "text-muted-foreground line-through",
                        )}
                      >
                        {t.title}
                      </span>

                      {t.description && (
                        <p className="line-clamp-2 text-xs leading-relaxed text-muted-foreground">
                          {t.description}
                        </p>
                      )}

                      {(t.priority !== "none" ||
                        t.labels.length > 0 ||
                        progress.total > 0 ||
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
                          {t.labels.map((l) => (
                            <LabelChip key={l.id} label={l} />
                          ))}
                          {progress.total > 0 && (
                            <span
                              className={cn(
                                "inline-flex items-center gap-1 rounded-full border border-border bg-background px-1.5 py-0.5 text-[11px] font-medium tabular-nums",
                                progress.done === progress.total
                                  ? "text-success"
                                  : "text-muted-foreground",
                              )}
                              title="Subtasks done"
                            >
                              <ListChecks size={11} />
                              {progress.done}/{progress.total}
                            </span>
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
          labels={labels}
          onClose={() => setEditing(null)}
          onPatch={patch}
          onDelete={remove}
          onArchive={archive}
          onSync={syncTask}
        />
      )}

      {labelsOpen && (
        <LabelManager
          labels={labels}
          onClose={() => setLabelsOpen(false)}
          onChanged={load}
        />
      )}

      {archivedOpen && (
        <ArchivedTasks
          onClose={() => setArchivedOpen(false)}
          onRestored={load}
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
        "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-0.5 text-xs font-medium transition-colors",
        active
          ? "border-transparent bg-primary text-primary-foreground"
          : "border-border bg-background text-muted-foreground hover:bg-muted hover:text-foreground",
      )}
    >
      {children}
    </button>
  );
}

function LabelChip({ label }: { label: Label }) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full px-1.5 py-0.5 text-[11px] font-medium",
        labelStyle(label.color).chip,
      )}
    >
      {label.name}
    </span>
  );
}

function ColorPicker({
  value,
  onChange,
}: {
  value: LabelColor;
  onChange: (c: LabelColor) => void;
}) {
  return (
    <div className="flex items-center gap-1" role="radiogroup" aria-label="Label color">
      {labelColors.map((c) => (
        <button
          key={c.key}
          type="button"
          role="radio"
          aria-checked={value === c.key}
          aria-label={c.key}
          title={c.key}
          onClick={() => onChange(c.key)}
          className={cn(
            "h-4 w-4 rounded-full ring-offset-2 ring-offset-card transition",
            c.dot,
            value === c.key ? "ring-2 ring-ring" : "opacity-70 hover:opacity-100",
          )}
        />
      ))}
    </div>
  );
}

function TaskEditor({
  task,
  labels,
  onClose,
  onPatch,
  onDelete,
  onArchive,
  onSync,
}: {
  task: Task;
  labels: Label[];
  onClose: () => void;
  onPatch: (id: number, body: Partial<Task>) => Promise<void>;
  onDelete: (id: number) => void;
  onArchive: (id: number) => void;
  onSync: (t: Task) => void;
}) {
  const [title, setTitle] = useState(task.title);
  const [description, setDescription] = useState(task.description);
  const [status, setStatus] = useState<Task["status"]>(task.status);
  const [priority, setPriority] = useState<Priority>(task.priority);
  const [due, setDue] = useState(task.due_at ? task.due_at.slice(0, 10) : "");
  const [saving, setSaving] = useState(false);
  const toast = useToast();

  const dirty =
    title !== task.title ||
    description !== task.description ||
    status !== task.status ||
    priority !== task.priority ||
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
        due_at: due ? new Date(due).toISOString() : null,
      } as Partial<Task>);
      onClose();
    } finally {
      setSaving(false);
    }
  }

  // Labels apply immediately (no Save needed), like the checklist below.
  async function toggleLabel(l: Label, attached: boolean) {
    try {
      onSync(
        attached
          ? await api.del<Task>(`/api/tasks/${task.id}/labels/${l.id}`)
          : await api.post<Task>(`/api/tasks/${task.id}/labels`, { label_id: l.id }),
      );
    } catch (err) {
      toast.error((err as Error).message);
    }
  }

  return (
    <Modal
      open
      onClose={onClose}
      icon={Pencil}
      title="Edit task"
      description="Labels and subtasks save as you go; other fields save with the button."
      className="sm:max-w-xl"
      footer={
        <>
          <Button
            variant="destructive"
            size="sm"
            onClick={() => {
              if (confirm("Permanently delete this task? This cannot be undone.")) onDelete(task.id);
            }}
          >
            <Trash2 size={14} /> Delete
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="mr-auto"
            onClick={() => onArchive(task.id)}
            title="Hide from the board; restore any time from Archived"
          >
            <Archive size={14} /> Archive
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
      <div className="space-y-3">
        <div className="space-y-1.5">
          <label className="flex h-5 items-center gap-1.5 text-xs font-medium text-muted-foreground">
            <Type size={13} /> Title
          </label>
          <Input
            value={title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Task title"
            autoFocus
          />
        </div>

        <div className="space-y-1.5">
          <label className="flex h-5 items-center gap-1.5 text-xs font-medium text-muted-foreground">
            <AlignLeft size={13} /> Description
          </label>
          <Textarea
            rows={4}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Add more detail, acceptance criteria, links…"
          />
        </div>

        <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
          <div className="space-y-1.5">
            <label className="flex h-5 items-center gap-1.5 text-xs font-medium text-muted-foreground">
              <Columns3 size={13} /> Status
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
            <label className="flex h-5 items-center gap-1.5 text-xs font-medium text-muted-foreground">
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
            <label className="flex h-5 items-center gap-1.5 text-xs font-medium text-muted-foreground">
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
                  className="shrink-0"
                  onClick={() => setDue("")}
                >
                  <X size={14} />
                </Button>
              )}
            </div>
          </div>
        </div>

        <div className="space-y-1.5">
          <label className="flex h-5 items-center gap-1.5 text-xs font-medium text-muted-foreground">
            <Tag size={13} /> Labels
          </label>
          {labels.length === 0 ? (
            <p className="text-xs text-muted-foreground">
              No labels yet. Create some with “Labels” at the top of the board.
            </p>
          ) : (
            <div className="flex flex-wrap gap-1.5">
              {labels.map((l) => {
                const on = task.labels.some((x) => x.id === l.id);
                return (
                  <button
                    key={l.id}
                    type="button"
                    aria-pressed={on}
                    onClick={() => toggleLabel(l, on)}
                    className={cn(
                      "inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-xs font-medium transition-colors",
                      on
                        ? cn("border-transparent", labelStyle(l.color).chip)
                        : "border-border bg-background text-muted-foreground hover:bg-muted",
                    )}
                  >
                    <span
                      className={cn("h-1.5 w-1.5 rounded-full", labelStyle(l.color).dot)}
                      aria-hidden
                    />
                    {l.name}
                    {on && <Check size={11} />}
                  </button>
                );
              })}
            </div>
          )}
        </div>

        <SubtaskList task={task} onSync={onSync} />
      </div>
    </Modal>
  );
}

// SubtaskList is the Task's one-level checklist. Every edit goes straight to
// the API and the returned parent Task is synced back, so the card's progress
// stays current. Checking every item never changes the Task's status.
function SubtaskList({
  task,
  onSync,
}: {
  task: Task;
  onSync: (t: Task) => void;
}) {
  const [newTitle, setNewTitle] = useState("");
  const [armed, setArmed] = useState<number | null>(null);
  const [dropIndex, setDropIndex] = useState<number | null>(null);
  const dragId = useRef<number | null>(null);
  const toast = useToast();
  const items = task.subtasks;
  const { done, total } = task.subtask_progress;

  async function run(p: Promise<Task>) {
    try {
      onSync(await p);
      return true;
    } catch (err) {
      toast.error((err as Error).message);
      return false;
    }
  }

  async function add(e: React.FormEvent) {
    e.preventDefault();
    const t = newTitle.trim();
    if (!t) return;
    if (await run(api.post<Task>(`/api/tasks/${task.id}/subtasks`, { title: t }))) {
      setNewTitle("");
    }
  }

  function rename(st: Subtask, value: string) {
    const t = value.trim();
    if (!t || t === st.title) return;
    run(api.patch<Task>(`/api/subtasks/${st.id}`, { title: t }));
  }

  function onItemDragOver(e: React.DragEvent, i: number) {
    e.preventDefault();
    const rect = e.currentTarget.getBoundingClientRect();
    setDropIndex(e.clientY - rect.top > rect.height / 2 ? i + 1 : i);
  }

  function endDrag() {
    dragId.current = null;
    setArmed(null);
    setDropIndex(null);
  }

  function onDrop() {
    const id = dragId.current;
    const target = dropIndex;
    endDrag();
    if (id == null || target == null) return;
    const from = items.findIndex((s) => s.id === id);
    const ids = items.map((s) => s.id).filter((x) => x !== id);
    ids.splice(target > from ? target - 1 : target, 0, id);
    if (ids.every((x, i) => x === items[i].id)) return;
    // Optimistic reorder so the row doesn't jump back while the request flies.
    onSync({
      ...task,
      subtasks: ids.map((x) => items.find((s) => s.id === x)!),
    });
    run(api.post<Task>(`/api/tasks/${task.id}/subtasks/reorder`, { ids }));
  }

  return (
    <div className="space-y-1.5">
      <label className="flex h-5 items-center gap-1.5 text-xs font-medium text-muted-foreground">
        <ListChecks size={13} /> Subtasks
        {total > 0 && (
          <span className="ml-auto tabular-nums">
            {done}/{total} done
          </span>
        )}
      </label>
      {total > 0 && (
        <div className="h-1 overflow-hidden rounded-full bg-muted">
          <div
            className="h-full rounded-full bg-success transition-[width]"
            style={{ width: `${(done / total) * 100}%` }}
          />
        </div>
      )}
      {items.length > 0 && (
        <ul onDragOver={(e) => e.preventDefault()} onDrop={onDrop}>
          {items.map((st, i) => (
            <li key={st.id}>
              {dropIndex === i && <div className="h-0.5 rounded bg-primary" />}
              <div
                draggable={armed === st.id}
                onDragStart={() => {
                  dragId.current = st.id;
                }}
                onDragEnd={endDrag}
                onDragOver={(e) => onItemDragOver(e, i)}
                onPointerUp={() => setArmed(null)}
                className="group flex items-center gap-2 rounded-md px-1 py-0.5 hover:bg-muted/50"
              >
                <GripVertical
                  size={13}
                  onPointerDown={() => setArmed(st.id)}
                  className="shrink-0 cursor-grab text-muted-foreground/40 transition group-hover:text-muted-foreground"
                  aria-hidden
                />
                <input
                  type="checkbox"
                  aria-label={`Mark “${st.title}” ${st.done ? "not done" : "done"}`}
                  className="h-4 w-4 shrink-0 cursor-pointer accent-primary"
                  checked={st.done}
                  onChange={() =>
                    run(api.patch<Task>(`/api/subtasks/${st.id}`, { done: !st.done }))
                  }
                />
                <input
                  key={`${st.id}:${st.title}`}
                  defaultValue={st.title}
                  aria-label="Subtask title"
                  onBlur={(e) => rename(st, e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") e.currentTarget.blur();
                    if (e.key === "Escape") {
                      e.stopPropagation();
                      e.currentTarget.value = st.title;
                      e.currentTarget.blur();
                    }
                  }}
                  className={cn(
                    "h-7 min-w-0 flex-1 rounded bg-transparent px-1 text-sm outline-none focus:bg-background focus:ring-1 focus:ring-ring/40",
                    st.done && "text-muted-foreground line-through",
                  )}
                />
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label="Delete subtask"
                  onClick={() => run(api.del<Task>(`/api/subtasks/${st.id}`))}
                  className="h-6 w-6 shrink-0 text-muted-foreground opacity-0 transition hover:text-destructive focus-visible:opacity-100 group-hover:opacity-100"
                >
                  <X size={13} />
                </Button>
              </div>
            </li>
          ))}
          {dropIndex === items.length && <div className="h-0.5 rounded bg-primary" />}
        </ul>
      )}
      <form onSubmit={add} className="flex gap-2">
        <Input
          className="h-8"
          placeholder="Add a subtask…"
          value={newTitle}
          onChange={(e) => setNewTitle(e.target.value)}
        />
        <Button type="submit" variant="outline" size="sm" disabled={!newTitle.trim()}>
          <Plus size={14} /> Add
        </Button>
      </form>
    </div>
  );
}

function LabelManager({
  labels,
  onClose,
  onChanged,
}: {
  labels: Label[];
  onClose: () => void;
  onChanged: () => void;
}) {
  const [name, setName] = useState("");
  const [color, setColor] = useState<LabelColor>("teal");
  const toast = useToast();

  async function create(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    try {
      await api.post("/api/labels", { name: name.trim(), color });
      setName("");
      onChanged();
      toast.success("Label created");
    } catch (err) {
      toast.error((err as Error).message);
    }
  }

  async function update(l: Label, body: Partial<Label>) {
    try {
      await api.patch(`/api/labels/${l.id}`, body);
      onChanged();
    } catch (err) {
      toast.error((err as Error).message);
      onChanged();
    }
  }

  async function remove(l: Label) {
    if (!confirm(`Delete label “${l.name}”? It is removed from its tasks; the tasks stay.`)) return;
    try {
      await api.del(`/api/labels/${l.id}`);
      onChanged();
      toast.success("Label deleted");
    } catch (err) {
      toast.error((err as Error).message);
    }
  }

  return (
    <Modal
      open
      onClose={onClose}
      icon={Tag}
      title="Labels"
      description="Group tasks across columns. Rename, recolor, or delete at any time."
    >
      <div className="space-y-4">
        <form onSubmit={create} className="space-y-2">
          <div className="flex gap-2">
            <Input
              placeholder="New label name…"
              value={name}
              onChange={(e) => setName(e.target.value)}
              autoFocus
            />
            <Button type="submit" disabled={!name.trim()}>
              <Plus size={15} /> Add
            </Button>
          </div>
          <ColorPicker value={color} onChange={setColor} />
        </form>

        {labels.length === 0 ? (
          <p className="py-4 text-center text-sm text-muted-foreground">
            No labels yet.
          </p>
        ) : (
          <ul className="divide-y divide-border rounded-md border border-border">
            {labels.map((l) => (
              <li key={l.id} className="flex flex-wrap items-center gap-3 px-3 py-2">
                <input
                  key={`${l.id}:${l.name}`}
                  defaultValue={l.name}
                  aria-label="Label name"
                  onBlur={(e) => {
                    const v = e.target.value.trim();
                    if (v && v !== l.name) update(l, { name: v });
                    else e.target.value = l.name;
                  }}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") e.currentTarget.blur();
                  }}
                  className={cn(
                    "h-7 min-w-[8rem] flex-1 rounded-full px-2.5 text-xs font-medium outline-none focus:ring-2 focus:ring-ring/40",
                    labelStyle(l.color).chip,
                  )}
                />
                <ColorPicker
                  value={l.color}
                  onChange={(c) => c !== l.color && update(l, { color: c })}
                />
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={`Delete label ${l.name}`}
                  onClick={() => remove(l)}
                  className="h-7 w-7 text-muted-foreground hover:text-destructive"
                >
                  <Trash2 size={14} />
                </Button>
              </li>
            ))}
          </ul>
        )}
      </div>
    </Modal>
  );
}

function ArchivedTasks({
  onClose,
  onRestored,
}: {
  onClose: () => void;
  onRestored: () => void;
}) {
  const [items, setItems] = useState<Task[] | null>(null);
  const toast = useToast();

  useEffect(() => {
    api
      .get<Task[]>("/api/tasks?archived=true")
      .then(setItems)
      .catch((err) => {
        toast.error((err as Error).message);
        setItems([]);
      });
  }, []);

  async function restore(t: Task) {
    try {
      await api.post(`/api/tasks/${t.id}/unarchive`);
      setItems((prev) => (prev ?? []).filter((x) => x.id !== t.id));
      onRestored();
      toast.success("Task restored to the board");
    } catch (err) {
      toast.error((err as Error).message);
    }
  }

  async function remove(t: Task) {
    if (!confirm("Permanently delete this task? This cannot be undone.")) return;
    try {
      await api.del(`/api/tasks/${t.id}`);
      setItems((prev) => (prev ?? []).filter((x) => x.id !== t.id));
      toast.success("Task deleted");
    } catch (err) {
      toast.error((err as Error).message);
    }
  }

  return (
    <Modal
      open
      onClose={onClose}
      icon={Archive}
      title="Archived tasks"
      description="Hidden from the board, newest first. Restore returns a task to the bottom of its column."
      className="sm:max-w-xl"
    >
      {items === null ? (
        <div className="flex justify-center py-8">
          <Spinner />
        </div>
      ) : items.length === 0 ? (
        <EmptyState
          icon={Archive}
          title="Nothing archived"
          description="Archive finished work from a card, its editor, or the Done column."
        />
      ) : (
        <ul className="divide-y divide-border rounded-md border border-border">
          {items.map((t) => (
            <li key={t.id} className="flex items-start gap-3 px-3 py-2.5">
              <div className="min-w-0 flex-1 space-y-1">
                <p className="truncate text-sm font-medium">{t.title}</p>
                <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-muted-foreground">
                  <Badge className="px-1.5 py-0 text-[11px]">
                    {columns.find((c) => c.key === t.status)?.label}
                  </Badge>
                  {t.labels.map((l) => (
                    <LabelChip key={l.id} label={l} />
                  ))}
                  {t.archived_at && (
                    <span>
                      Archived{" "}
                      {new Date(t.archived_at).toLocaleDateString(undefined, {
                        month: "short",
                        day: "numeric",
                        year: "numeric",
                      })}
                    </span>
                  )}
                </div>
              </div>
              <Button variant="outline" size="xs" onClick={() => restore(t)}>
                <ArchiveRestore size={13} /> Restore
              </Button>
              <Button
                variant="ghost"
                size="icon-sm"
                aria-label="Delete permanently"
                title="Delete permanently"
                onClick={() => remove(t)}
                className="h-7 w-7 text-muted-foreground hover:text-destructive"
              >
                <Trash2 size={14} />
              </Button>
            </li>
          ))}
        </ul>
      )}
    </Modal>
  );
}
