import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import {
  Button,
  Card,
  Input,
  Badge,
  Textarea,
  Select,
  PageHeader,
  EmptyState,
} from "@/components/ui";
import { cn } from "@/lib/api";
import {
  FileText,
  FileImage,
  FileVideo,
  FileAudio,
  FileSpreadsheet,
  FileCode,
  FileArchive,
  File,
  Presentation,
  FolderPlus,
  FolderInput,
  RefreshCw,
  Trash2,
  Download,
  X,
  Tag,
  Folder,
  AlertTriangle,
  Search,
  type LucideIcon,
} from "lucide-react";

type Root = { id: number; path: string };
type Resource = {
  id: number;
  root_id: number;
  path: string;
  name: string;
  size: number;
  is_dir: boolean;
  modified_at: string | null;
  state: "present" | "missing";
  note: string;
  tags: string[];
};

function bytes(n: number): string {
  const units = ["B", "KB", "MB", "GB"];
  let v = n;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toFixed(0)} ${units[i]}`;
}

// A compact, human modified-date: time today, "Yesterday", a weekday this week,
// "Mon D" this year, else "Mon D, YYYY" — so the column scans at a glance.
function modified(iso: string | null): string {
  if (!iso) return "—";
  const d = new Date(iso);
  const now = new Date();
  const startOfDay = (t: Date) =>
    new Date(t.getFullYear(), t.getMonth(), t.getDate()).getTime();
  const dayDiff = Math.round((startOfDay(now) - startOfDay(d)) / 86400000);
  if (dayDiff === 0)
    return d.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
  if (dayDiff === 1) return "Yesterday";
  if (dayDiff > 1 && dayDiff < 7)
    return d.toLocaleDateString(undefined, { weekday: "short" });
  if (d.getFullYear() === now.getFullYear())
    return d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
  return d.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

/* --------------------------------------------------- File-type filtering */

// A small, Finder/Drive-style taxonomy. Resources carry no mime type from the
// Host, so we classify client-side by directory flag + lowercase extension.
type FileKind =
  | "folder"
  | "document"
  | "image"
  | "video"
  | "audio"
  | "spreadsheet"
  | "presentation"
  | "code"
  | "archive"
  | "other";

// Extensions per kind. Order of the `kinds` array below drives the filter order
// and the fallback is "other".
const kindExtensions: Record<Exclude<FileKind, "folder" | "other">, string[]> = {
  document: ["pdf", "doc", "docx", "txt", "md", "markdown", "rtf", "odt", "pages", "tex", "epub"],
  image: ["jpg", "jpeg", "png", "gif", "webp", "svg", "bmp", "ico", "heic", "heif", "tiff", "tif", "avif"],
  video: ["mp4", "mov", "avi", "mkv", "webm", "flv", "wmv", "m4v", "mpeg", "mpg", "3gp"],
  audio: ["mp3", "wav", "flac", "aac", "ogg", "oga", "m4a", "wma", "opus", "aiff"],
  spreadsheet: ["xls", "xlsx", "csv", "tsv", "ods", "numbers"],
  presentation: ["ppt", "pptx", "odp", "key"],
  code: [
    "js", "jsx", "ts", "tsx", "go", "py", "rb", "php", "java", "kt", "c", "h", "cc",
    "cpp", "hpp", "cs", "rs", "swift", "sh", "bash", "zsh", "html", "htm", "css",
    "scss", "sass", "less", "json", "yaml", "yml", "toml", "xml", "sql", "vue",
    "svelte", "lua", "pl", "r", "dart", "gradle", "ini", "conf", "env",
  ],
  archive: ["zip", "tar", "gz", "tgz", "bz2", "xz", "rar", "7z", "zst", "lz", "lzma"],
};

const extToKind = new Map<string, FileKind>();
for (const [kind, exts] of Object.entries(kindExtensions)) {
  for (const ext of exts) extToKind.set(ext, kind as FileKind);
}

// Extension-less filenames that still read as code/config.
const nameToKind: Record<string, FileKind> = {
  dockerfile: "code",
  makefile: "code",
  rakefile: "code",
};

function extensionOf(name: string): string {
  const dot = name.lastIndexOf(".");
  if (dot <= 0) return ""; // no extension, or a dotfile like ".env"
  return name.slice(dot + 1).toLowerCase();
}

function classify(r: Resource): FileKind {
  if (r.is_dir) return "folder";
  const named = nameToKind[r.name.toLowerCase()];
  if (named) return named;
  return extToKind.get(extensionOf(r.name)) ?? "other";
}

// Display metadata per kind. Array order == filter order. `chip` is the tinted
// icon-chip (bg + fg) used everywhere a file icon appears, so a file's type is
// legible at a glance in both light and dark themes.
const kinds: { key: FileKind; label: string; icon: LucideIcon; chip: string }[] = [
  { key: "folder", label: "Folders", icon: Folder, chip: "bg-amber-500/15 text-amber-600 dark:text-amber-400" },
  { key: "document", label: "Documents", icon: FileText, chip: "bg-blue-500/15 text-blue-600 dark:text-blue-400" },
  { key: "image", label: "Images", icon: FileImage, chip: "bg-violet-500/15 text-violet-600 dark:text-violet-400" },
  { key: "video", label: "Videos", icon: FileVideo, chip: "bg-rose-500/15 text-rose-600 dark:text-rose-400" },
  { key: "audio", label: "Audio", icon: FileAudio, chip: "bg-pink-500/15 text-pink-600 dark:text-pink-400" },
  { key: "spreadsheet", label: "Spreadsheets", icon: FileSpreadsheet, chip: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400" },
  { key: "presentation", label: "Presentations", icon: Presentation, chip: "bg-orange-500/15 text-orange-600 dark:text-orange-400" },
  { key: "code", label: "Code", icon: FileCode, chip: "bg-cyan-500/15 text-cyan-600 dark:text-cyan-400" },
  { key: "archive", label: "Archives", icon: FileArchive, chip: "bg-yellow-500/15 text-yellow-700 dark:text-yellow-500" },
  { key: "other", label: "Other", icon: File, chip: "bg-muted text-muted-foreground" },
];

const kindMeta = (k: FileKind) => kinds.find((x) => x.key === k) ?? kinds[kinds.length - 1];

// The directory a resource lives in (its absolute path minus the file name),
// shown as a subtle second line so same-named files in different folders are
// distinguishable.
function parentDir(r: Resource): string {
  const trimmed = r.path.endsWith("/") ? r.path.slice(0, -1) : r.path;
  const cut = trimmed.length - r.name.length - 1;
  if (cut <= 0) return "";
  return trimmed.slice(0, cut);
}

export default function Files() {
  const [roots, setRoots] = useState<Root[]>([]);
  const [resources, setResources] = useState<Resource[]>([]);
  const [newRoot, setNewRoot] = useState("");
  const [rootFilter, setRootFilter] = useState<number | "">("");
  const [tagFilter, setTagFilter] = useState("");
  const [kindFilter, setKindFilter] = useState<FileKind | "all">("all");
  const [selected, setSelected] = useState<Resource | null>(null);
  const [checked, setChecked] = useState<Set<number>>(new Set());
  const [batchTag, setBatchTag] = useState("");
  const [batchDest, setBatchDest] = useState("");
  const [batchBusy, setBatchBusy] = useState(false);
  const [error, setError] = useState("");
  const [scanning, setScanning] = useState<number | null>(null);

  // Resources after applying the file-type filter (root/tag filters are applied
  // server-side; the kind filter is client-side since the Host sends no type).
  function visible(): Resource[] {
    if (kindFilter === "all") return resources;
    return resources.filter((r) => classify(r) === kindFilter);
  }

  // How many resources fall into each kind, for the filter chip counts.
  function kindCounts(): Map<FileKind, number> {
    const counts = new Map<FileKind, number>();
    for (const r of resources) {
      const k = classify(r);
      counts.set(k, (counts.get(k) ?? 0) + 1);
    }
    return counts;
  }

  async function loadRoots() {
    setRoots(await api.get<Root[]>("/api/files/roots"));
  }
  async function loadResources() {
    const params = new URLSearchParams();
    if (rootFilter !== "") params.set("root_id", String(rootFilter));
    if (tagFilter) params.set("tag", tagFilter);
    const q = params.toString();
    setResources(await api.get<Resource[]>(`/api/files/resources${q ? `?${q}` : ""}`));
  }

  useEffect(() => {
    loadRoots();
  }, []);
  useEffect(() => {
    loadResources();
  }, [rootFilter, tagFilter]);

  // Drop checked ids that no longer exist in the current resource set so the
  // batch bar's count always matches reality after loads/deletes.
  useEffect(() => {
    setChecked((prev) => {
      if (prev.size === 0) return prev;
      const live = new Set(resources.map((r) => r.id));
      let changed = false;
      const next = new Set<number>();
      for (const id of prev) {
        if (live.has(id)) next.add(id);
        else changed = true;
      }
      return changed ? next : prev;
    });
  }, [resources]);

  async function addRoot(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    try {
      await api.post("/api/files/roots", { path: newRoot });
      setNewRoot("");
      loadRoots();
    } catch (err) {
      setError((err as Error).message);
    }
  }

  async function removeRoot(id: number) {
    await api.del(`/api/files/roots/${id}`);
    loadRoots();
    loadResources();
  }

  async function scan(id: number) {
    setScanning(id);
    try {
      await api.post(`/api/files/roots/${id}/scan`);
      loadResources();
    } finally {
      setScanning(null);
    }
  }

  async function addTag(r: Resource, tag: string) {
    if (!tag.trim()) return;
    const updated = await api.post<Resource>(`/api/files/resources/${r.id}/tags`, { tag });
    setSelected(updated);
    loadResources();
  }
  async function removeTag(r: Resource, tag: string) {
    const updated = await api.del<Resource>(
      `/api/files/resources/${r.id}/tags/${encodeURIComponent(tag)}`,
    );
    setSelected(updated);
    loadResources();
  }
  async function saveNote(r: Resource, note: string) {
    const updated = await api.put<Resource>(`/api/files/resources/${r.id}/note`, { note });
    setSelected(updated);
    loadResources();
  }
  async function clearMissing(r: Resource) {
    await api.del(`/api/files/resources/${r.id}`);
    setSelected(null);
    loadResources();
  }
  async function rename(r: Resource, name: string) {
    const updated = await api.post<Resource>(`/api/files/resources/${r.id}/rename`, { name });
    setSelected(updated);
    loadResources();
  }
  async function move(r: Resource, dest: string) {
    try {
      const updated = await api.post<Resource>(`/api/files/resources/${r.id}/move`, { dest });
      setSelected(updated);
      loadResources();
    } catch (err) {
      setError((err as Error).message);
    }
  }
  async function deleteFile(r: Resource) {
    await api.del(`/api/files/resources/${r.id}/file`);
    setSelected(null);
    loadResources();
  }

  // ---- Multi-select / batch operations -------------------------------------
  // `checked` holds resource ids. We keep it pruned to currently-visible rows so
  // the batch bar never acts on files hidden by a filter the user forgot about.
  function toggleChecked(id: number) {
    setChecked((prev) => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  }
  function clearChecked() {
    setChecked(new Set());
  }
  function toggleAllVisible() {
    const vis = visible();
    const allOn = vis.length > 0 && vis.every((r) => checked.has(r.id));
    setChecked(allOn ? new Set() : new Set(vis.map((r) => r.id)));
  }

  // The checked resources, resolved against the current list and in table order.
  function checkedResources(): Resource[] {
    return visible().filter((r) => checked.has(r.id));
  }
  // Just the checked ids, for the single-request batch endpoints.
  function checkedIDs(resources: Resource[]): number[] {
    return resources.map((r) => r.id);
  }

  async function batchAddTag() {
    const tag = batchTag.trim();
    const ids = checkedIDs(checkedResources());
    if (!tag || ids.length === 0) return;
    setBatchBusy(true);
    setError("");
    try {
      // One request tags every selected resource server-side.
      await api.post("/api/files/resources/batch/tag", { ids, tag });
      setBatchTag("");
      await loadResources();
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBatchBusy(false);
    }
  }

  function batchDownload() {
    // Trigger a download per selected (non-dir, present) file.
    for (const r of checkedResources()) {
      if (r.is_dir || r.state === "missing") continue;
      const a = document.createElement("a");
      a.href = `/api/files/resources/${r.id}/download`;
      a.download = r.name;
      document.body.appendChild(a);
      a.click();
      a.remove();
    }
  }

  async function batchMove() {
    const dest = batchDest.trim();
    if (!dest) return;
    // Missing files have no on-disk source to move, so skip them.
    const ids = checkedIDs(checkedResources().filter((r) => r.state !== "missing"));
    if (ids.length === 0) return;
    setBatchBusy(true);
    setError("");
    try {
      // One request moves every selected file into the destination directory.
      const res = await api.post<{ failed?: string[] }>(
        "/api/files/resources/batch/move",
        { ids, dest },
      );
      setBatchDest("");
      if (selected && checked.has(selected.id)) setSelected(null);
      await loadResources();
      if (res?.failed?.length) {
        setError(`Could not move ${res.failed.length} file(s).`);
      }
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBatchBusy(false);
    }
  }

  async function batchDelete() {
    const targets = checkedResources();
    const ids = checkedIDs(targets);
    if (ids.length === 0) return;
    const missing = targets.filter((r) => r.state === "missing").length;
    const live = targets.length - missing;
    if (
      !confirm(
        `Permanently delete ${live} file(s)` +
          (missing ? ` and clear ${missing} missing entr(ies)` : "") +
          "? This cannot be undone.",
      )
    )
      return;
    setBatchBusy(true);
    setError("");
    try {
      // One request deletes the bytes and clears the index rows server-side.
      const res = await api.post<{ failed?: string[] }>(
        "/api/files/resources/batch/delete",
        { ids },
      );
      if (selected && checked.has(selected.id)) setSelected(null);
      clearChecked();
      await loadResources();
      if (res?.failed?.length) {
        setError(`Could not delete ${res.failed.length} file(s).`);
      }
    } catch (err) {
      setError((err as Error).message);
    } finally {
      setBatchBusy(false);
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        icon={FileText}
        title="Files"
        description="Manage Resources across your Managed Roots."
      />

      <Card className="space-y-3 p-4">
        <div className="flex items-center gap-2">
          <Folder size={16} className="text-muted-foreground" />
          <h2 className="font-semibold">Managed Roots</h2>
        </div>
        <form onSubmit={addRoot} className="flex gap-2">
          <Input
            placeholder="/absolute/path/to/directory"
            className="font-mono"
            value={newRoot}
            onChange={(e) => setNewRoot(e.target.value)}
          />
          <Button type="submit" disabled={!newRoot.trim()}>
            <FolderPlus size={15} /> Add root
          </Button>
        </form>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <div className="space-y-1.5">
          {roots.map((r) => (
            <div
              key={r.id}
              className="flex items-center justify-between gap-2 rounded-md border border-border bg-muted/30 px-3 py-2 text-sm"
            >
              <span className="truncate font-mono text-xs">{r.path}</span>
              <div className="flex shrink-0 gap-1">
                <Button
                  size="sm"
                  variant="outline"
                  loading={scanning === r.id}
                  onClick={() => scan(r.id)}
                >
                  <RefreshCw size={13} /> Scan
                </Button>
                <Button
                  size="icon-sm"
                  variant="ghost"
                  aria-label="Remove root"
                  className="text-muted-foreground hover:text-destructive"
                  onClick={() => removeRoot(r.id)}
                >
                  <Trash2 size={14} />
                </Button>
              </div>
            </div>
          ))}
          {roots.length === 0 && (
            <p className="text-sm text-muted-foreground">
              No roots configured. Add a directory above to index it.
            </p>
          )}
        </div>
      </Card>

      <div className="flex flex-wrap items-center gap-2">
        <div className="relative max-w-xs flex-1">
          <Search
            size={15}
            className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            className="pl-9"
            placeholder="Filter by tag…"
            value={tagFilter}
            onChange={(e) => setTagFilter(e.target.value)}
          />
          {tagFilter && (
            <button
              type="button"
              onClick={() => setTagFilter("")}
              aria-label="Clear tag filter"
              className="absolute right-2 top-1/2 flex h-5 w-5 -translate-y-1/2 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            >
              <X size={13} />
            </button>
          )}
        </div>
        <Select
          value={rootFilter}
          onChange={(e) =>
            setRootFilter(e.target.value === "" ? "" : Number(e.target.value))
          }
        >
          <option value="">All roots</option>
          {roots.map((r) => (
            <option key={r.id} value={r.id}>
              {r.path}
            </option>
          ))}
        </Select>
      </div>

      {resources.length > 0 && (
        <div className="flex flex-wrap items-center gap-1.5">
          <span className="mr-1 text-xs font-medium text-muted-foreground">
            Type:
          </span>
          <TypeChip
            active={kindFilter === "all"}
            onClick={() => setKindFilter("all")}
            count={resources.length}
          >
            All
          </TypeChip>
          {(() => {
            const counts = kindCounts();
            return kinds.map((k) => {
              const count = counts.get(k.key) ?? 0;
              if (count === 0) return null;
              const Icon = k.icon;
              return (
                <TypeChip
                  key={k.key}
                  active={kindFilter === k.key}
                  onClick={() => setKindFilter(k.key)}
                  count={count}
                  icon={<Icon size={13} />}
                >
                  {k.label}
                </TypeChip>
              );
            });
          })()}
        </div>
      )}

      {checked.size > 0 && (
        <Card className="flex flex-wrap items-center gap-2 border-primary/40 bg-accent/40 p-3">
          <span className="flex items-center gap-2 text-sm font-medium">
            <span className="flex h-6 min-w-6 items-center justify-center rounded-full bg-primary px-1.5 text-xs font-semibold tabular-nums text-primary-foreground">
              {checked.size}
            </span>
            selected
          </span>
          <div className="h-5 w-px bg-border" />
          <div className="flex items-center gap-1.5">
            <Input
              className="h-8 w-36"
              placeholder="add tag…"
              value={batchTag}
              onChange={(e) => setBatchTag(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  batchAddTag();
                }
              }}
            />
            <Button
              size="sm"
              variant="outline"
              loading={batchBusy}
              disabled={!batchTag.trim()}
              onClick={batchAddTag}
            >
              <Tag size={13} /> Tag
            </Button>
          </div>
          <div className="flex items-center gap-1.5">
            <Input
              className="h-8 w-48 font-mono"
              placeholder="/managed/root/dest…"
              value={batchDest}
              onChange={(e) => setBatchDest(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  batchMove();
                }
              }}
            />
            <Button
              size="sm"
              variant="outline"
              loading={batchBusy}
              disabled={!batchDest.trim()}
              onClick={batchMove}
            >
              <FolderInput size={13} /> Move
            </Button>
          </div>
          <Button size="sm" variant="outline" onClick={batchDownload}>
            <Download size={13} /> Download
          </Button>
          <Button
            size="sm"
            variant="destructive"
            loading={batchBusy}
            onClick={batchDelete}
          >
            <Trash2 size={13} /> Delete
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="ml-auto"
            onClick={clearChecked}
          >
            <X size={13} /> Clear
          </Button>
        </Card>
      )}

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card className="overflow-hidden p-0 lg:col-span-2">
          {visible().length === 0 ? (
            <EmptyState
              icon={Search}
              title="No matches"
              description={
                resources.length === 0
                  ? "Add a Managed Root above and scan it to index its files."
                  : "No Resources match the current filters. Try clearing them."
              }
              action={
                resources.length > 0 ? (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      setTagFilter("");
                      setRootFilter("");
                      setKindFilter("all");
                    }}
                  >
                    Clear filters
                  </Button>
                ) : undefined
              }
            />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full border-separate border-spacing-0 text-sm">
                <thead>
                  <tr className="text-left text-xs text-muted-foreground">
                    <th className="sticky top-0 z-10 w-10 border-b border-border bg-card/95 px-4 py-2.5 backdrop-blur">
                      <input
                        type="checkbox"
                        aria-label="Select all"
                        className="h-4 w-4 cursor-pointer accent-primary align-middle"
                        checked={
                          visible().length > 0 &&
                          visible().every((r) => checked.has(r.id))
                        }
                        ref={(el) => {
                          if (el) {
                            const vis = visible();
                            const some = vis.some((r) => checked.has(r.id));
                            const all =
                              vis.length > 0 && vis.every((r) => checked.has(r.id));
                            el.indeterminate = some && !all;
                          }
                        }}
                        onChange={toggleAllVisible}
                      />
                    </th>
                    <th className="sticky top-0 z-10 border-b border-border bg-card/95 px-4 py-2.5 font-medium backdrop-blur">
                      Name
                    </th>
                    <th className="sticky top-0 z-10 border-b border-border bg-card/95 px-4 py-2.5 text-right font-medium backdrop-blur">
                      Size
                    </th>
                    <th className="sticky top-0 z-10 hidden border-b border-border bg-card/95 px-4 py-2.5 font-medium backdrop-blur sm:table-cell">
                      Modified
                    </th>
                    <th className="sticky top-0 z-10 border-b border-border bg-card/95 px-4 py-2.5 font-medium backdrop-blur">
                      Tags
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {visible().map((r) => (
                    <ResourceRow
                      key={r.id}
                      resource={r}
                      selected={selected?.id === r.id}
                      checked={checked.has(r.id)}
                      onToggleCheck={toggleChecked}
                      onSelect={setSelected}
                    />
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </Card>

        {selected && (
          <Detail
            key={selected.id}
            resource={selected}
            onAddTag={addTag}
            onRemoveTag={removeTag}
            onSaveNote={saveNote}
            onClearMissing={clearMissing}
            onRename={rename}
            onMove={move}
            onDelete={deleteFile}
            onClose={() => setSelected(null)}
          />
        )}
      </div>
    </div>
  );
}

// A single file-type filter chip: optional leading icon, label, and a count.
function TypeChip({
  active,
  onClick,
  count,
  icon,
  children,
}: {
  active: boolean;
  onClick: () => void;
  count: number;
  icon?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={active}
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-xs font-medium transition-colors",
        active
          ? "border-transparent bg-primary text-primary-foreground"
          : "border-border bg-background text-muted-foreground hover:bg-muted hover:text-foreground",
      )}
    >
      {icon}
      {children}
      <span
        className={cn(
          "tabular-nums",
          active ? "text-primary-foreground/70" : "text-muted-foreground/70",
        )}
      >
        {count}
      </span>
    </button>
  );
}

// One file row in the table. A tinted type-chip plus the parent directory make
// each entry scannable and disambiguate same-named files across folders.
function ResourceRow({
  resource,
  selected,
  checked,
  onToggleCheck,
  onSelect,
}: {
  resource: Resource;
  selected: boolean;
  checked: boolean;
  onToggleCheck: (id: number) => void;
  onSelect: (r: Resource) => void;
}) {
  const meta = kindMeta(classify(resource));
  const Icon = meta.icon;
  const dir = parentDir(resource);
  return (
    <tr
      onClick={() => onSelect(resource)}
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onSelect(resource);
        }
      }}
      className={cn(
        "group cursor-pointer outline-none transition-colors",
        selected ? "bg-accent/60" : "hover:bg-muted/50 focus-visible:bg-muted/50",
        resource.state === "missing" && "opacity-60",
      )}
    >
      <td
        className="border-b border-border/60 px-4 py-2.5"
        onClick={(e) => e.stopPropagation()}
      >
        <input
          type="checkbox"
          aria-label={`Select ${resource.name}`}
          className="h-4 w-4 cursor-pointer accent-primary align-middle"
          checked={checked}
          onChange={() => onToggleCheck(resource.id)}
        />
      </td>
      <td
        className={cn(
          "border-b border-border/60 px-4 py-2.5",
          selected && "border-l-2 border-l-primary pl-[14px]",
        )}
      >
        <div className="flex items-center gap-3">
          <span
            className={cn(
              "flex h-8 w-8 shrink-0 items-center justify-center rounded-md",
              meta.chip,
            )}
          >
            <Icon size={16} />
          </span>
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="truncate font-medium text-foreground">
                {resource.name}
              </span>
              {resource.state === "missing" && (
                <Badge variant="destructive">missing</Badge>
              )}
            </div>
            {dir && (
              <span className="block truncate font-mono text-xs text-muted-foreground">
                {dir}
              </span>
            )}
          </div>
        </div>
      </td>
      <td className="border-b border-border/60 px-4 py-2.5 text-right tabular-nums text-muted-foreground">
        {resource.is_dir ? "—" : bytes(resource.size)}
      </td>
      <td className="hidden border-b border-border/60 px-4 py-2.5 tabular-nums text-muted-foreground sm:table-cell">
        {modified(resource.modified_at)}
      </td>
      <td className="border-b border-border/60 px-4 py-2.5">
        <div className="flex flex-wrap gap-1">
          {resource.tags.map((t) => (
            <Badge key={t} variant="primary">
              {t}
            </Badge>
          ))}
        </div>
      </td>
    </tr>
  );
}

function Detail({
  resource,
  onAddTag,
  onRemoveTag,
  onSaveNote,
  onClearMissing,
  onRename,
  onMove,
  onDelete,
  onClose,
}: {
  resource: Resource;
  onAddTag: (r: Resource, tag: string) => void;
  onRemoveTag: (r: Resource, tag: string) => void;
  onSaveNote: (r: Resource, note: string) => void;
  onClearMissing: (r: Resource) => void;
  onRename: (r: Resource, name: string) => void;
  onMove: (r: Resource, dest: string) => void;
  onDelete: (r: Resource) => void;
  onClose: () => void;
}) {
  const [tag, setTag] = useState("");
  const [note, setNote] = useState(resource.note);
  const [name, setName] = useState(resource.name);
  const [dest, setDest] = useState("");
  const meta = kindMeta(classify(resource));
  const Icon = meta.icon;

  return (
    <Card className="h-fit animate-scale-in space-y-4 p-4 lg:sticky lg:top-8">
      <div className="flex items-start justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2.5">
          <span
            className={cn(
              "flex h-9 w-9 shrink-0 items-center justify-center rounded-md",
              meta.chip,
            )}
          >
            <Icon size={18} />
          </span>
          <h2 className="truncate font-semibold">{resource.name}</h2>
        </div>
        <Button size="icon-sm" variant="ghost" aria-label="Close" onClick={onClose}>
          <X size={16} />
        </Button>
      </div>
      <div className="break-all rounded-md bg-muted/50 p-2 font-mono text-xs text-muted-foreground">
        {resource.path}
      </div>

      {resource.state === "missing" ? (
        <div className="space-y-3 rounded-md border border-destructive/30 bg-destructive/5 p-3">
          <p className="flex items-center gap-2 text-sm text-destructive">
            <AlertTriangle size={15} />
            This file is missing from disk.
          </p>
          <Button size="sm" variant="outline" onClick={() => onClearMissing(resource)}>
            Clear from index
          </Button>
        </div>
      ) : (
        <>
          <a
            href={`/api/files/resources/${resource.id}/download`}
            className="inline-flex h-8 w-full items-center justify-center gap-2 rounded-md bg-muted px-3 text-sm font-medium text-foreground transition-colors hover:bg-muted/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
          >
            <Download size={15} /> Download / open
          </a>

          <div className="space-y-1.5">
            <label className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
              <Tag size={13} /> Tags
            </label>
            <div className="flex flex-wrap gap-1">
              {resource.tags.map((t) => (
                <Badge
                  key={t}
                  variant="primary"
                  className="cursor-pointer transition hover:bg-destructive/15 hover:text-destructive"
                  onClick={() => onRemoveTag(resource, t)}
                >
                  {t}
                  <X size={11} />
                </Badge>
              ))}
              {resource.tags.length === 0 && (
                <span className="text-xs text-muted-foreground">No tags yet.</span>
              )}
            </div>
            <div className="flex gap-2">
              <Input
                className="h-8"
                placeholder="add tag"
                value={tag}
                onChange={(e) => setTag(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    onAddTag(resource, tag);
                    setTag("");
                  }
                }}
              />
              <Button
                size="sm"
                onClick={() => {
                  onAddTag(resource, tag);
                  setTag("");
                }}
              >
                Add
              </Button>
            </div>
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-medium text-muted-foreground">Note</label>
            <Textarea rows={3} value={note} onChange={(e) => setNote(e.target.value)} />
            <Button size="sm" variant="outline" onClick={() => onSaveNote(resource, note)}>
              Save note
            </Button>
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-medium text-muted-foreground">Rename</label>
            <div className="flex gap-2">
              <Input className="h-8" value={name} onChange={(e) => setName(e.target.value)} />
              <Button size="sm" onClick={() => onRename(resource, name)}>
                Rename
              </Button>
            </div>
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-medium text-muted-foreground">
              Move to (absolute path)
            </label>
            <div className="flex gap-2">
              <Input
                className="h-8 font-mono"
                placeholder="/managed/root/dest"
                value={dest}
                onChange={(e) => setDest(e.target.value)}
              />
              <Button size="sm" onClick={() => onMove(resource, dest)}>
                Move
              </Button>
            </div>
          </div>

          <div className="border-t border-border pt-3">
            <Button
              size="sm"
              variant="destructive"
              className="w-full"
              onClick={() => {
                if (confirm("Permanently delete this file?")) onDelete(resource);
              }}
            >
              <Trash2 size={14} /> Delete file (permanent)
            </Button>
          </div>
        </>
      )}
    </Card>
  );
}
