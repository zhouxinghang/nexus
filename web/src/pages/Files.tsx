import { Fragment, lazy, Suspense, useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { api } from "@/lib/api";
import { useToast } from "@/lib/toast";
import {
  Button,
  Card,
  Input,
  Badge,
  Select,
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
  Eye,
  ChevronRight,
  ArrowUp,
  ArrowDown,
  ArrowUpDown,
  Settings2,
  SlidersHorizontal,
  Layers,
  Info,
  type LucideIcon,
} from "lucide-react";

const FilePreview = lazy(() => import("@/components/FilePreview"));

type Root = { id: number; path: string };
type Resource = {
  id: number;
  root_id: number;
  path: string;
  name: string;
  size: number;
  is_dir: boolean;
  modified_at: string | null;
  file_created_at: string | null;
  state: "present" | "missing";
  kind: FileKind;
  tags: string[];
};

type Directory = {
  path: string;
  name: string;
  modified_at: string | null;
  file_created_at: string | null;
  state: "present" | "missing";
};
type SortBy = "name" | "size" | "modified" | "created";
type SortOrder = "asc" | "desc";
type BrowseMode = "folder" | "type";

// One Files page: the loaded rows, the server-side totals, and per-kind counts
// (keyed by FileKind) for the current root/tag filters, which drive the type
// chips independently of the active kind filter.
type ResourcePage = {
  items: Resource[];
  total: number;
  limit: number;
  offset: number;
  kinds: Record<string, number>;
  directories: Directory[];
};

// Only the type view is paginated. Folder requests return complete children.
const RESOURCE_PAGE = 200;

// Live scan progress reported by the scan status endpoint. A scan is started
// with POST (202) and polled with GET until it leaves the "running" phase.
type ScanStatus = {
  root_id: number;
  phase: "idle" | "running" | "completed" | "failed";
  total: number;
  indexed: number;
  removed: number;
  error?: string;
};

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

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

// A compact file date: time today, "Yesterday", a weekday this week,
// "Mon D" this year, else "Mon D, YYYY" — so the column scans at a glance.
function fileDate(iso: string | null): string {
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

// A small, Finder/Drive-style taxonomy. The Host sends no mime type, so the
// server classifies each Resource (see the resource_kind SQL function) and
// returns the bucket as `resource.kind`; this type just mirrors those values
// so the client can label, icon, and filter by them.
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
  const [searchParams, setSearchParams] = useSearchParams();
  const [roots, setRoots] = useState<Root[]>([]);
  const [resources, setResources] = useState<Resource[]>([]);
  const [directories, setDirectories] = useState<Directory[]>([]);
  const [total, setTotal] = useState(0);
  const [kindCounts, setKindCounts] = useState<Record<string, number>>({});
  const [loading, setLoading] = useState(false);
  const [loadError, setLoadError] = useState("");
  const requestVersion = useRef(0);
  const [newRoot, setNewRoot] = useState("");
  const [manageRoots, setManageRoots] = useState(false);
  const [showFilters, setShowFilters] = useState(false);
  const [showDetail, setShowDetail] = useState(false);
  const [activeDirectory, setActiveDirectory] = useState("");
  const mode: BrowseMode = searchParams.get("mode") === "type" ? "type" : "folder";
  const rootParam = searchParams.get("root");
  const rootFilter = rootParam && rootParam !== "all" ? Number(rootParam) : "";
  const currentRoot = roots.find((r) => r.id === rootFilter);
  const currentDir = searchParams.get("dir") || currentRoot?.path || "";
  const sortParam = searchParams.get("sort");
  const sortBy: SortBy = sortParam === "size" || sortParam === "modified" || sortParam === "created" ? sortParam : "name";
  const sortOrder: SortOrder = searchParams.get("order") === "desc" ? "desc" : "asc";
  const tagFilter = searchParams.get("tag") || "";
  const kindParam = searchParams.get("kind");
  const kindFilter = kinds.some((k) => k.key === kindParam) ? kindParam as FileKind : "all";
  const nameFilter = searchParams.get("name") || "";
  const folderFilter = searchParams.get("folder") || "";
  const pageParam = Number(searchParams.get("page") || "1");
  const pageNumber = Number.isSafeInteger(pageParam) && pageParam > 0 && pageParam <= 10_000_000 ? pageParam : 1;
  const [nameDraft, setNameDraft] = useState(nameFilter);
  const [tagDraft, setTagDraft] = useState(tagFilter);
  const [folderDraft, setFolderDraft] = useState(folderFilter);
  const [selected, setSelected] = useState<Resource | null>(null);
  const [previewing, setPreviewing] = useState<Resource | null>(null);
  const [checked, setChecked] = useState<Set<number>>(new Set());
  const [batchTag, setBatchTag] = useState("");
  const [batchDest, setBatchDest] = useState("");
  const [batchBusy, setBatchBusy] = useState(false);
  const [batchAction, setBatchAction] = useState<"tag" | "move" | null>(null);
  const tableRef = useRef<HTMLDivElement>(null);
  const selectionAnchor = useRef<number | null>(null);
  const toast = useToast();
  const [scanning, setScanning] = useState<number | null>(null);
  // Live progress for the scanning root: files seen vs. indexed so far. Null
  // until the first status poll returns.
  const [scanProgress, setScanProgress] = useState<ScanStatus | null>(null);

  // Type counts include the current location/name/tag filters, not just this page.
  const totalAcrossKinds = Object.values(kindCounts).reduce((a, b) => a + b, 0);
  const resourcePaths = new Set(resources.map((r) => r.path));
  const visibleDirectories = directories.filter((d) => !resourcePaths.has(d.path));
  const hasFilters = !!(nameFilter || tagFilter || folderFilter || kindFilter !== "all");
  const groups = mode === "type" && kindFilter === "all"
    ? kinds.map((kind) => ({ kind, items: resources.filter((r) => r.kind === kind.key) })).filter((group) => group.items.length > 0)
    : [{ kind: null, items: resources }];
  const displayResources = groups.flatMap((group) => group.items);

  async function loadRoots(initial = false) {
    try {
      const items = await api.get<Root[]>("/api/files/roots");
      setRoots(items);
      if (initial && items[0]) {
        setSearchParams((current) => {
          if (current.has("root") || current.get("mode") === "type") return current;
          const next = new URLSearchParams(current);
          next.set("root", String(items[0].id));
          next.delete("dir");
          return next;
        }, { replace: true });
      }
    } catch (err) {
      toast.error((err as Error).message);
    }
  }
  function browse(rootID: number | "", dir?: string) {
    setSelected(null);
    clearChecked();
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      next.set("root", rootID === "" ? "all" : String(rootID));
      if (dir) next.set("dir", dir);
      else next.delete("dir");
      next.delete("page");
      if (mode === "type") next.delete("folder");
      return next;
    });
  }
  function switchMode(nextMode: BrowseMode) {
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      next.set("mode", nextMode);
      next.delete("page");
      if (nextMode === "folder" && rootFilter === "" && roots[0]) {
        next.set("root", String(roots[0].id));
        next.delete("dir");
      }
      return next;
    });
  }
  function setQuery(values: Record<string, string>, replace = false) {
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      for (const [key, value] of Object.entries(values)) {
        if (value) next.set(key, value);
        else next.delete(key);
      }
      if (!("page" in values)) next.delete("page");
      return next;
    }, { replace });
  }
  function clearFilters() {
    setQuery({ name: "", tag: "", folder: "", kind: "" });
  }
  function goUp() {
    if (!currentRoot || currentDir === currentRoot.path) return;
    const parent = currentDir.slice(0, currentDir.lastIndexOf("/")) || "/";
    browse(currentRoot.id, parent);
  }
  function changeSort(key: SortBy) {
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      next.set("sort", key);
      next.set("order", key === sortBy ? (sortOrder === "asc" ? "desc" : "asc") : (key === "name" ? "asc" : "desc"));
      next.delete("page");
      return next;
    });
  }
  function resourcesPath() {
    const params = new URLSearchParams();
    params.set("mode", mode);
    if (rootFilter !== "") params.set("root_id", String(rootFilter));
    if (mode === "folder") {
      if (currentDir) params.set("parent", currentDir);
    } else {
      if (folderFilter) params.set("dir", folderFilter);
      if (nameFilter) params.set("name", nameFilter);
      if (tagFilter) params.set("tag", tagFilter);
      if (kindFilter !== "all") params.set("kind", kindFilter);
      params.set("offset", String((pageNumber - 1) * RESOURCE_PAGE));
      params.set("limit", String(RESOURCE_PAGE));
    }
    params.set("sort", sortBy);
    params.set("order", sortOrder);
    return `/api/files/resources?${params}`;
  }
  // Obsolete responses must not replace the directory/sort currently visible.
  const queryKey = resourcesPath();
  const canLoad = mode === "type" || !!(currentRoot && currentDir);
  const activeQuery = useRef(queryKey);
  activeQuery.current = queryKey;
  // One request per view/page. Version and scope guards reject late responses.
  async function loadResources(reset = false) {
    if (queryKey !== activeQuery.current) return;
    const version = ++requestVersion.current;
    setLoadError("");
    if (mode === "folder" && (!currentRoot || !currentDir)) {
      setResources([]);
      setDirectories([]);
      setTotal(0);
      setKindCounts({});
      setLoading(false);
      return;
    }
    setLoading(true);
    if (reset) {
      setResources([]);
      setDirectories([]);
      setTotal(0);
      setKindCounts({});
      clearChecked();
    }
    try {
      const page = await api.get<ResourcePage>(queryKey);
      if (version !== requestVersion.current || queryKey !== activeQuery.current) return;
      if (mode === "type" && pageNumber > 1 && (pageNumber - 1) * RESOURCE_PAGE >= page.total) {
        setQuery({ page: String(Math.max(1, Math.ceil(page.total / RESOURCE_PAGE))) }, true);
        return;
      }
      setResources(page.items);
      setDirectories(page.directories ?? []);
      setTotal(page.total);
      setKindCounts(page.kinds);
    } catch (err) {
      if (version === requestVersion.current && queryKey === activeQuery.current) {
        setLoadError((err as Error).message);
      }
    } finally {
      if (version === requestVersion.current && queryKey === activeQuery.current) setLoading(false);
    }
  }
  const refreshResources = useRef(loadResources);
  refreshResources.current = loadResources;

  useEffect(() => {
    void loadRoots(true);
  }, []);
  useEffect(() => {
    loadResources(true);
    // Fetch the current view/page whenever its server-side scope changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [queryKey, canLoad]);

  useEffect(() => {
    setSelected(null);
    setActiveDirectory("");
    setShowDetail(false);
    selectionAnchor.current = null;
    clearChecked();
  }, [queryKey]);

  useEffect(() => { setNameDraft(nameFilter); }, [nameFilter]);
  useEffect(() => { setTagDraft(tagFilter); setFolderDraft(folderFilter); }, [tagFilter, folderFilter]);
  useEffect(() => {
    if (mode !== "type" || nameDraft.trim() === nameFilter) return;
    const timer = window.setTimeout(() => setQuery({ name: nameDraft.trim() }, true), 300);
    return () => window.clearTimeout(timer);
  }, [nameDraft, nameFilter, mode]);

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
    try {
      const root = await api.post<Root>("/api/files/roots", { path: newRoot });
      setNewRoot("");
      await loadRoots();
      browse(root.id);
      toast.success("Managed Root added");
    } catch (err) {
      toast.error((err as Error).message);
    }
  }

  async function removeRoot(id: number) {
    try {
      await api.del(`/api/files/roots/${id}`);
      if (rootFilter === id) browse("");
      void loadRoots();
      toast.success("Managed Root removed");
    } catch (err) {
      toast.error((err as Error).message);
    }
  }

  // scan starts a background scan, then polls the status endpoint until it
  // finishes. The scan runs server-side off the request, so a huge directory
  // no longer holds one long request open; the button stays "scanning" and
  // shows live progress until the backend reports done.
  async function scan(id: number) {
    setScanning(id);
    setScanProgress(null);
    try {
      const started = await api.post<ScanStatus>(`/api/files/roots/${id}/scan`);
      let status = started;
      setScanProgress(started);
      while (status.phase === "running") {
        await sleep(400);
        status = await api.get<ScanStatus>(`/api/files/roots/${id}/scan`);
        setScanProgress(status);
      }
      if (status.phase === "failed") {
        throw new Error(status.error || "scan failed");
      }
      void refreshResources.current();
      const summary =
        status.removed > 0
          ? `Rescan complete — ${status.indexed} indexed, ${status.removed} missing`
          : `Rescan complete — ${status.indexed} indexed`;
      toast.success(summary);
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setScanning(null);
      setScanProgress(null);
    }
  }

  async function addTag(r: Resource, tag: string) {
    if (!tag.trim()) return;
    try {
      const updated = await api.post<Resource>(`/api/files/resources/${r.id}/tags`, { tag });
      setSelected((current) => current?.id === updated.id ? updated : current);
      loadResources();
    } catch (err) {
      toast.error((err as Error).message);
    }
  }
  async function removeTag(r: Resource, tag: string) {
    try {
      const updated = await api.del<Resource>(
        `/api/files/resources/${r.id}/tags/${encodeURIComponent(tag)}`,
      );
      setSelected((current) => current?.id === updated.id ? updated : current);
      loadResources();
    } catch (err) {
      toast.error((err as Error).message);
    }
  }
  async function clearMissing(r: Resource) {
    try {
      await api.del(`/api/files/resources/${r.id}`);
      setSelected(null);
      loadResources();
      toast.success("Missing Resource cleared");
    } catch (err) {
      toast.error((err as Error).message);
    }
  }
  async function rename(r: Resource, name: string) {
    try {
      const updated = await api.post<Resource>(`/api/files/resources/${r.id}/rename`, { name });
      setSelected((current) => current?.id === updated.id ? updated : current);
      loadResources();
      toast.success("File renamed");
    } catch (err) {
      toast.error((err as Error).message);
    }
  }
  async function move(r: Resource, dest: string) {
    try {
      const updated = await api.post<Resource>(`/api/files/resources/${r.id}/move`, { dest });
      setSelected((current) => current?.id === updated.id ? updated : current);
      loadResources();
      toast.success("File moved");
    } catch (err) {
      toast.error((err as Error).message);
    }
  }
  async function deleteFile(r: Resource) {
    try {
      await api.del(`/api/files/resources/${r.id}/file`);
      setSelected(null);
      loadResources();
      toast.success("File deleted");
    } catch (err) {
      toast.error((err as Error).message);
    }
  }

  // ---- Multi-select / batch operations -------------------------------------
  // `checked` holds resource ids. All loaded rows are already server-filtered
  // by root/tag/kind, so the batch bar acts exactly on what the Owner sees.
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
  function selectResource(resource: Resource, event?: React.MouseEvent) {
    setSelected(resource);
    setActiveDirectory("");
    if (event?.shiftKey && selectionAnchor.current !== null) {
      const from = displayResources.findIndex((r) => r.id === selectionAnchor.current);
      const to = displayResources.findIndex((r) => r.id === resource.id);
      if (from >= 0) {
        const range = displayResources.slice(Math.min(from, to), Math.max(from, to) + 1);
        setChecked(new Set(range.map((r) => r.id)));
      }
    } else if (event?.metaKey || event?.ctrlKey) {
      const next = new Set(checked);
      if (checked.size === 0 && selected) next.add(selected.id);
      next.has(resource.id) ? next.delete(resource.id) : next.add(resource.id);
      setChecked(next);
      setSelected(next.has(resource.id) ? resource : null);
      selectionAnchor.current = resource.id;
    } else {
      clearChecked();
      selectionAnchor.current = resource.id;
    }
  }
  function onTableKeyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    const target = event.target as HTMLElement;
    if (target.closest("input, select, textarea")) return;
    if ((event.metaKey || event.ctrlKey) && event.key === "ArrowUp" && mode === "folder") {
      event.preventDefault();
      goUp();
      return;
    }
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "a") {
      event.preventDefault();
      toggleAllVisible();
      return;
    }
    if (event.key !== "ArrowUp" && event.key !== "ArrowDown") return;
    const rows = Array.from(tableRef.current?.querySelectorAll<HTMLElement>("[data-file-row]") ?? []);
    const index = rows.findIndex((row) => row.contains(target));
    const next = rows[index + (event.key === "ArrowDown" ? 1 : -1)];
    if (next) {
      event.preventDefault();
      next.focus();
      const resource = resources.find((r) => String(r.id) === next.dataset.resourceId);
      if (resource) selectResource(resource);
      else {
        setSelected(null);
        clearChecked();
        setActiveDirectory(next.dataset.directoryPath || "");
      }
    }
  }
  function toggleAllVisible() {
    const allOn = resources.length > 0 && resources.every((r) => checked.has(r.id));
    setChecked(allOn ? new Set() : new Set(resources.map((r) => r.id)));
  }

  // The checked resources, resolved against the current list and in table order.
  function checkedResources(): Resource[] {
    return displayResources.filter((r) => checked.has(r.id));
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
    try {
      // One request tags every selected resource server-side.
      await api.post("/api/files/resources/batch/tag", { ids, tag });
      setBatchTag("");
      setBatchAction(null);
      await loadResources();
      toast.success(`Tagged ${ids.length} Resource(s)`);
    } catch (err) {
      toast.error((err as Error).message);
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
    try {
      // One request moves every selected file into the destination directory.
      const res = await api.post<{ failed?: string[] }>(
        "/api/files/resources/batch/move",
        { ids, dest },
      );
      setBatchDest("");
      setBatchAction(null);
      if (selected && checked.has(selected.id)) setSelected(null);
      await loadResources();
      if (res?.failed?.length) {
        toast.error(`Could not move ${res.failed.length} file(s).`);
      } else {
        toast.success(`Moved ${ids.length} file(s)`);
      }
    } catch (err) {
      toast.error((err as Error).message);
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
        toast.error(`Could not delete ${res.failed.length} file(s).`);
      } else {
        toast.success(`Deleted ${ids.length} item(s)`);
      }
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setBatchBusy(false);
    }
  }

  return (
    <div className="space-y-4">
      <header className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap items-center gap-4">
          <h1 className="text-2xl font-semibold tracking-tight">Files</h1>
          <div role="group" aria-label="Browsing mode" className="inline-flex rounded-lg bg-muted p-1">
            <Button size="sm" variant="ghost" aria-pressed={mode === "folder"} onClick={() => switchMode("folder")} className={cn("gap-1.5", mode === "folder" && "bg-card shadow-subtle hover:bg-card")}>
              <Folder size={15} aria-hidden /> Folders
            </Button>
            <Button size="sm" variant="ghost" aria-pressed={mode === "type"} onClick={() => switchMode("type")} className={cn("gap-1.5", mode === "type" && "bg-card shadow-subtle hover:bg-card")}>
              <Layers size={15} aria-hidden /> By type
            </Button>
          </div>
        </div>
        <Button variant="ghost" size="sm" onClick={() => setManageRoots(true)}>
          <Settings2 size={15} aria-hidden /> Manage roots
        </Button>
      </header>

      <Card className="overflow-hidden shadow-none">
        <div className="flex min-h-[min(34rem,70vh)]">
          <aside aria-label="Files sidebar" className="hidden w-44 shrink-0 space-y-5 border-r border-border bg-muted/20 p-3 md:block">
            <nav aria-label="Managed Roots" className="space-y-1">
              <p className="px-2 py-1 text-xs font-medium text-muted-foreground">Locations</p>
              <Button variant="ghost" size="sm" className={cn("w-full justify-start", rootFilter === "" && "bg-accent text-accent-foreground")} onClick={() => browse("")} aria-current={rootFilter === "" ? "location" : undefined}>
                <Layers size={15} aria-hidden /> All roots
              </Button>
              {roots.map((root) => (
                <button key={root.id} type="button" title={root.path} aria-label={`Browse ${root.path}`} aria-current={rootFilter === root.id ? "location" : undefined} onClick={() => browse(root.id)} className={cn("flex min-h-9 w-full min-w-0 items-center gap-2 rounded-md px-3 text-left text-sm transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring", rootFilter === root.id && "bg-accent text-accent-foreground")}>
                  <Folder size={15} aria-hidden className="shrink-0" />
                  <span className="truncate">{root.path.replace(/\/$/, "").split("/").pop() || "/"}</span>
                </button>
              ))}
            </nav>
            {mode === "type" && (
              <nav aria-label="Resource types" className="space-y-1">
                <p className="px-2 py-1 text-xs font-medium text-muted-foreground">Types</p>
                <TypeChip active={kindFilter === "all"} onClick={() => setQuery({ kind: "" })} count={totalAcrossKinds}>All types</TypeChip>
                {kinds.filter((kind) => kindCounts[kind.key] || kindFilter === kind.key).map((kind) => {
                  const Icon = kind.icon;
                  return <TypeChip key={kind.key} active={kindFilter === kind.key} onClick={() => setQuery({ kind: kind.key })} count={kindCounts[kind.key] || 0} icon={<Icon size={15} aria-hidden />}>{kind.label}</TypeChip>;
                })}
              </nav>
            )}
          </aside>

          <div className="flex min-w-0 flex-1 flex-col">
            <div className="flex flex-wrap items-center gap-2 border-b border-border p-3">
              {mode === "type" && <Select aria-label="Managed Root" className="w-full min-w-0 max-w-full md:hidden" value={rootFilter} onChange={(e) => browse(e.target.value === "" ? "" : Number(e.target.value))}>
                <option value="">All roots</option>
                {roots.map((root) => <option key={root.id} value={root.id}>{root.path}</option>)}
              </Select>}
              {mode === "folder" ? (
                <div className="flex min-w-0 flex-1 basis-full items-center gap-1 md:basis-auto">
                  <Button size="icon" variant="ghost" aria-label="Up to parent folder" title="Up to parent folder (⌘/Ctrl ↑)" disabled={!currentRoot || currentDir === currentRoot.path} onClick={goUp}><ArrowUp size={16} aria-hidden /></Button>
                  <nav aria-label="Directory breadcrumbs" className="flex min-w-0 flex-1 flex-wrap items-center gap-0.5 text-sm">
                    <Button size="sm" variant="ghost" onClick={() => browse("")}>All roots</Button>
                    {currentRoot && [{ name: currentRoot.path, path: currentRoot.path }, ...currentDir
                      .slice(currentRoot.path === "/" ? 1 : currentRoot.path.length + 1)
                      .split("/").filter(Boolean).map((name, i, parts) => ({ name, path: `${currentRoot.path.replace(/\/$/, "")}/${parts.slice(0, i + 1).join("/")}` }))].map((crumb) => (
                        <span key={crumb.path} className="flex min-w-0 items-center gap-0.5">
                          <ChevronRight size={13} aria-hidden className="shrink-0 text-muted-foreground" />
                          <Button size="sm" variant="ghost" title={crumb.path} className="max-w-[min(16rem,48vw)] px-2" aria-current={crumb.path === currentDir ? "location" : undefined} onClick={() => browse(currentRoot.id, crumb.path)}><span className="truncate">{crumb.name}</span></Button>
                        </span>
                      ))}
                  </nav>
                </div>
              ) : (
                <>
                  <div className="relative min-w-36 flex-1">
                    <Search size={15} aria-hidden className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" />
                    <Input type="search" aria-label="Filter by name" placeholder="Search names…" className="pl-9" value={nameDraft} onChange={(e) => setNameDraft(e.target.value)} onKeyDown={(e) => { if (e.key === "Enter") setQuery({ name: nameDraft.trim() }); }} />
                  </div>
                  <Button size="sm" variant={showFilters || tagFilter || folderFilter ? "subtle" : "ghost"} aria-expanded={showFilters} aria-controls="files-filters" onClick={() => setShowFilters(!showFilters)}>
                    <SlidersHorizontal size={15} aria-hidden /> Filters{(tagFilter || folderFilter) && <span className="text-xs"> · {[tagFilter, folderFilter].filter(Boolean).length}</span>}
                  </Button>
                  <Select aria-label="Resource type" className="max-w-full md:hidden" value={kindFilter} onChange={(e) => setQuery({ kind: e.target.value === "all" ? "" : e.target.value })}>
                    <option value="all">All types</option>
                    {kinds.map((kind) => <option key={kind.key} value={kind.key}>{kind.label}</option>)}
                  </Select>
                </>
              )}
              <div className="ml-auto flex items-center gap-1">
                <Select aria-label="Sort by" className="w-auto" value={sortBy} onChange={(e) => setQuery({ sort: e.target.value, order: e.target.value === "name" ? "asc" : "desc" })}>
                  <option value="name">Name</option><option value="size">Size</option><option value="modified">Modified</option><option value="created">Created</option>
                </Select>
                <Button size="icon" variant="ghost" aria-label={`Sort ${sortOrder === "asc" ? "descending" : "ascending"}`} onClick={() => changeSort(sortBy)}>{sortOrder === "asc" ? <ArrowUp size={15} aria-hidden /> : <ArrowDown size={15} aria-hidden />}</Button>
                <Button size="icon" variant="ghost" aria-label="Refresh files" disabled={loading} onClick={() => loadResources()}><RefreshCw size={15} aria-hidden /></Button>
                <Button size="icon" variant={showDetail ? "subtle" : "ghost"} aria-label="Show file details" title="File details" aria-expanded={showDetail && !!selected} disabled={!selected} onClick={() => setShowDetail(!showDetail)}><Info size={16} aria-hidden /></Button>
              </div>
            </div>

            {mode === "type" && showFilters && (
              <form id="files-filters" className="flex flex-wrap items-end gap-3 border-b border-border bg-muted/20 p-3" onSubmit={(e) => { e.preventDefault(); setQuery({ name: nameDraft.trim(), tag: tagDraft.trim(), folder: folderDraft.trim() }); }}>
                <label className="min-w-36 flex-1 space-y-1 text-xs font-medium text-muted-foreground">Folder
                  <Input className="font-mono" placeholder="/absolute/folder/path" value={folderDraft} onChange={(e) => setFolderDraft(e.target.value)} />
                </label>
                <label className="min-w-28 flex-1 space-y-1 text-xs font-medium text-muted-foreground">Tag
                  <Input placeholder="Exact tag" value={tagDraft} onChange={(e) => setTagDraft(e.target.value)} />
                </label>
                <Button size="sm" type="submit" variant="outline">Apply</Button>
              </form>
            )}
            {mode === "type" && hasFilters && (
              <div aria-label="Active filters" className="flex flex-wrap items-center gap-2 border-b border-border px-3 py-2 text-xs text-muted-foreground">
                {nameFilter && <span className="max-w-full truncate" title={nameFilter}>Name: {nameFilter}</span>}
                {folderFilter && <span className="max-w-full truncate" title={folderFilter}>Folder: {folderFilter}</span>}
                {tagFilter && <span className="max-w-full truncate" title={tagFilter}>Tag: {tagFilter}</span>}
                {kindFilter !== "all" && <span>Type: {kindMeta(kindFilter).label}</span>}
                <Button size="xs" variant="ghost" onClick={clearFilters} className="ml-auto"><X size={13} aria-hidden /> Clear filters</Button>
              </div>
            )}


            <div className="flex min-w-0 flex-1 flex-col lg:flex-row">
              <div className="min-w-0 flex-1" aria-busy={loading}>
                {loadError ? (
                  <div role="alert" className="space-y-3 p-6"><p className="text-sm text-destructive">{loadError}</p><Button size="sm" variant="outline" onClick={() => loadResources(true)}>Retry</Button></div>
                ) : loading ? (
                  <div role="status" className="flex min-h-56 items-center justify-center gap-2 text-sm text-muted-foreground"><RefreshCw size={16} aria-hidden className="animate-spin motion-reduce:animate-none" /> Loading {mode === "folder" ? "directory" : "Resources"}…</div>
                ) : mode === "folder" && !currentRoot ? (
                  roots.length ? <div className="grid grid-cols-1 gap-2 p-4 sm:grid-cols-2">{roots.map((root) => <Button key={root.id} variant="ghost" className="h-auto min-w-0 justify-start gap-3 p-4" onClick={() => browse(root.id)}><Folder size={24} aria-hidden className="shrink-0 text-amber-600 dark:text-amber-400" /><span className="truncate" title={root.path}>{root.path}</span></Button>)}</div>
                    : <EmptyState icon={FolderPlus} title="Add a Managed Root" description="Choose a Host directory to start browsing." className="m-4" action={<Button size="sm" onClick={() => setManageRoots(true)}>Add root</Button>} />
                ) : resources.length === 0 && directories.length === 0 ? (
                  <EmptyState icon={mode === "folder" ? Folder : Search} title={mode === "folder" ? "Empty directory" : "No matches"} description={mode === "folder" ? "Scan the Managed Root to include new files." : hasFilters ? "Try a different filter or clear the current filters." : "Scan a Managed Root to index Resources."} className="m-4" action={mode === "type" && hasFilters ? <Button size="sm" variant="outline" onClick={clearFilters}>Clear filters</Button> : undefined} />
                ) : (
                  <div ref={tableRef} onKeyDown={onTableKeyDown} className="files-table-container max-h-[65vh] overflow-auto">
                    <table aria-label={mode === "folder" ? "Directory contents" : "Resources by type"} className="w-full table-fixed border-separate border-spacing-0 text-sm">
                      <thead>
                        <tr className="text-left text-xs text-muted-foreground">
                          <th className="sticky top-0 z-10 w-12 border-b border-border bg-card px-3 py-2">
                            <label className="inline-flex h-6 w-6 cursor-pointer items-center justify-center">
                              <input type="checkbox" aria-label="Select all" className="h-4 w-4 cursor-pointer accent-primary" checked={resources.length > 0 && resources.every((r) => checked.has(r.id))} ref={(el) => { if (el) el.indeterminate = resources.some((r) => checked.has(r.id)) && !resources.every((r) => checked.has(r.id)); }} onChange={toggleAllVisible} />
                            </label>
                          </th>
                          <th aria-sort={sortBy === "name" ? (sortOrder === "asc" ? "ascending" : "descending") : "none"} className="sticky top-0 z-10 border-b border-border bg-card px-3 py-2 font-medium"><SortHeader label="Name" sortKey="name" sortBy={sortBy} order={sortOrder} onSort={changeSort} /></th>
                          <th aria-sort={sortBy === "size" ? (sortOrder === "asc" ? "ascending" : "descending") : "none"} className="sticky top-0 z-10 w-20 border-b border-border bg-card px-3 py-2 text-right font-medium"><SortHeader label="Size" sortKey="size" sortBy={sortBy} order={sortOrder} onSort={changeSort} /></th>
                          <th aria-sort={sortBy === "modified" ? (sortOrder === "asc" ? "ascending" : "descending") : "none"} className="files-modified-column sticky top-0 z-10 w-28 border-b border-border bg-card px-3 py-2 font-medium"><SortHeader label="Modified" sortKey="modified" sortBy={sortBy} order={sortOrder} onSort={changeSort} /></th>
                          <th aria-sort={sortBy === "created" ? (sortOrder === "asc" ? "ascending" : "descending") : "none"} className="files-created-column sticky top-0 z-10 w-28 border-b border-border bg-card px-3 py-2 font-medium"><SortHeader label="Created" sortKey="created" sortBy={sortBy} order={sortOrder} onSort={changeSort} /></th>
                          <th className="files-tags-column sticky top-0 z-10 w-[20%] border-b border-border bg-card px-3 py-2 font-medium">Tags</th>
                        </tr>
                      </thead>
                      <tbody>
                        {visibleDirectories.map((dir) => <DirectoryRow key={dir.path} directory={dir} selected={activeDirectory === dir.path} onSelect={() => { setActiveDirectory(dir.path); setSelected(null); clearChecked(); }} onOpen={() => browse(currentRoot!.id, dir.path)} />)}
                        {groups.map((group) => (
                          <Fragment key={group.kind?.key || "all"}>
                            {group.kind && <tr><th scope="rowgroup" colSpan={6} className="border-b border-border bg-muted/30 px-3 py-2 text-left text-xs font-medium text-muted-foreground">{group.kind.label}<span className="ml-2 font-normal">{group.items.length} on this page</span></th></tr>}
                            {group.items.map((resource) => <ResourceRow key={resource.id} resource={resource} selected={selected?.id === resource.id} checked={checked.has(resource.id)} showPath={mode === "type"} onToggleCheck={toggleChecked} onSelect={selectResource} onPreview={setPreviewing} onOpenDirectory={(r) => { if (mode === "type") { setSearchParams({ mode: "folder", root: String(r.root_id), dir: r.path }); } else browse(r.root_id, r.path); }} />)}
                          </Fragment>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
              {selected && showDetail && <div className="w-full shrink-0 border-t border-border lg:w-72 lg:border-l lg:border-t-0"><Detail key={selected.id} resource={selected} onAddTag={addTag} onRemoveTag={removeTag} onClearMissing={clearMissing} onRename={rename} onMove={move} onDelete={deleteFile} onPreview={() => setPreviewing(selected)} onClose={() => setShowDetail(false)} /></div>}
            </div>

            {checked.size > 0 && (
              <div className="flex flex-wrap items-center gap-1 border-t border-border bg-accent/40 px-3 py-2">
                <span className="mr-2 text-sm font-medium">{checked.size} selected</span>
                <Button size="sm" variant="ghost" disabled={batchBusy} onClick={() => setBatchAction("tag")}><Tag size={14} aria-hidden /> Tag</Button>
                <Button size="sm" variant="ghost" disabled={batchBusy} onClick={() => setBatchAction("move")}><FolderInput size={14} aria-hidden /> Move</Button>
                <Button size="sm" variant="ghost" disabled={batchBusy} onClick={batchDownload}><Download size={14} aria-hidden /> Download</Button>
                <Button size="sm" variant="ghost" className="text-destructive hover:text-destructive" loading={batchBusy} onClick={batchDelete}><Trash2 size={14} aria-hidden /> Delete</Button>
                <Button size="icon-sm" variant="ghost" className="ml-auto" aria-label="Clear selection" disabled={batchBusy} onClick={clearChecked}><X size={14} aria-hidden /></Button>
              </div>
            )}

            {!loading && !loadError && (
              <footer className="mt-auto flex flex-wrap items-center justify-between gap-2 border-t border-border px-3 py-2 text-xs text-muted-foreground">
                {mode === "folder" ? <span>{currentRoot ? visibleDirectories.length + resources.length : roots.length} items</span> : <>
                  <span>{total ? `${((pageNumber - 1) * RESOURCE_PAGE + 1).toLocaleString()}–${((pageNumber - 1) * RESOURCE_PAGE + resources.length).toLocaleString()} of ${total.toLocaleString()}` : "0 Resources"}</span>
                  <div className="flex items-center gap-2">
                    <Button variant="ghost" size="xs" disabled={pageNumber <= 1} onClick={() => setQuery({ page: String(pageNumber - 1) })}>Previous</Button>
                    <span className="tabular-nums">{pageNumber} / {Math.max(1, Math.ceil(total / RESOURCE_PAGE))}</span>
                    <Button variant="ghost" size="xs" disabled={pageNumber * RESOURCE_PAGE >= total} onClick={() => setQuery({ page: String(pageNumber + 1) })}>Next</Button>
                  </div>
                </>}
              </footer>
            )}
          </div>
        </div>
      </Card>

      {manageRoots && <FilesDialog title="Managed Roots" onClose={() => setManageRoots(false)}>
        <form onSubmit={addRoot} className="mb-4 flex flex-wrap items-end gap-2">
          <label className="min-w-40 flex-1 space-y-1 text-xs font-medium text-muted-foreground">Host directory
            <Input placeholder="/absolute/path/to/directory" aria-label="Managed Root path" className="font-mono" value={newRoot} onChange={(e) => setNewRoot(e.target.value)} />
          </label>
          <Button type="submit" disabled={!newRoot.trim()}><FolderPlus size={15} aria-hidden /> Add root</Button>
        </form>
        <div className="space-y-2">
          {roots.map((root) => <div key={root.id} className="flex flex-wrap items-center gap-2 rounded-md border border-border p-3">
            <span className="min-w-0 flex-1 break-all font-mono text-xs">{root.path}</span>
            {scanning === root.id && scanProgress && <span role="status" className="text-xs tabular-nums text-muted-foreground">{scanProgress.indexed}/{scanProgress.total}</span>}
            <Button size="sm" variant="outline" loading={scanning === root.id} disabled={scanning !== null} onClick={() => scan(root.id)}><RefreshCw size={13} aria-hidden /> Scan</Button>
            <Button size="icon-sm" variant="ghost" aria-label={`Remove root ${root.path}`} className="text-muted-foreground hover:text-destructive" onClick={() => { if (confirm(`Remove ${root.path} from Nexus? Its index and tags will be removed; Host files will not be deleted.`)) removeRoot(root.id); }}><Trash2 size={14} aria-hidden /></Button>
          </div>)}
          {!roots.length && <p className="text-sm text-muted-foreground">No roots configured. Add a directory to index it.</p>}
        </div>
      </FilesDialog>}
      {batchAction && <FilesDialog title={batchAction === "tag" ? `Tag ${checked.size} Resources` : `Move ${checked.size} Resources`} onClose={() => { if (!batchBusy) setBatchAction(null); }}>
        <form className="space-y-4" onSubmit={(e) => { e.preventDefault(); if (!batchBusy) void (batchAction === "tag" ? batchAddTag() : batchMove()); }}>
          <label className="block space-y-1 text-sm">{batchAction === "tag" ? "Tag" : "Destination directory (absolute path)"}
            <Input value={batchAction === "tag" ? batchTag : batchDest} onChange={(e) => batchAction === "tag" ? setBatchTag(e.target.value) : setBatchDest(e.target.value)} disabled={batchBusy} />
          </label>
          <div className="flex justify-end gap-2"><Button type="button" variant="ghost" disabled={batchBusy} onClick={() => setBatchAction(null)}>Cancel</Button><Button type="submit" loading={batchBusy} disabled={!(batchAction === "tag" ? batchTag : batchDest).trim()}>{batchAction === "tag" ? "Add tag" : "Move"}</Button></div>
        </form>
      </FilesDialog>}
      {previewing && (
        <Suspense fallback={null}>
          <FilePreview
            resource={previewing}
            siblings={displayResources}
            onNavigate={(r) => {
              const next = resources.find((item) => item.id === r.id);
              if (next) setPreviewing(next);
            }}
            onClose={() => setPreviewing(null)}
            onSaved={() => {
              void loadResources();
              void api.get<Resource>(`/api/files/resources/${previewing.id}`).then((updated) => {
                setSelected((current) => current?.id === updated.id ? updated : current);
              }).catch((err) => toast.error((err as Error).message));
            }}
          />
        </Suspense>
      )}
    </div>
  );
}

// A native dialog provides modal focus containment and restores its trigger.
function FilesDialog({ title, onClose, children }: {
  title: string;
  onClose: () => void;
  children: React.ReactNode;
}) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const closeRef = useRef(onClose);
  closeRef.current = onClose;
  useEffect(() => {
    const trigger = document.activeElement as HTMLElement | null;
    const dialog = dialogRef.current!;
    dialog.showModal();
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      dialog.close();
      document.body.style.overflow = previousOverflow;
      trigger?.focus();
    };
  }, []);
  return (
    <dialog
      ref={dialogRef}
      aria-labelledby="files-dialog-title"
      className="max-h-[85dvh] w-[calc(100%-2rem)] max-w-xl overflow-auto rounded-xl border border-border bg-card p-0 text-card-foreground shadow-pop backdrop:bg-foreground/40"
      onCancel={(event) => { event.preventDefault(); closeRef.current(); }}
      onClick={(event) => {
        if (event.target !== event.currentTarget) return;
        const rect = event.currentTarget.getBoundingClientRect();
        if (event.clientX < rect.left || event.clientX > rect.right || event.clientY < rect.top || event.clientY > rect.bottom) closeRef.current();
      }}
    >
      <div className="flex items-center justify-between border-b border-border px-5 py-3">
        <h2 id="files-dialog-title" className="font-semibold">{title}</h2>
        <Button size="icon-sm" variant="ghost" aria-label="Close dialog" onClick={onClose}><X size={16} aria-hidden /></Button>
      </div>
      <div className="p-5">{children}</div>
    </dialog>
  );
}

function SortHeader({ label, sortKey, sortBy, order, onSort }: {
  label: string;
  sortKey: SortBy;
  sortBy: SortBy;
  order: SortOrder;
  onSort: (key: SortBy) => void;
}) {
  const Icon = sortBy !== sortKey ? ArrowUpDown : order === "asc" ? ArrowUp : ArrowDown;
  return (
    <button type="button" onClick={() => onSort(sortKey)} className="inline-flex min-h-6 items-center gap-1 rounded font-medium hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
      {label}<Icon size={12} aria-hidden />
    </button>
  );
}

function DirectoryRow({ directory, selected, onSelect, onOpen }: {
  directory: Directory;
  selected: boolean;
  onSelect: () => void;
  onOpen: () => void;
}) {
  return (
    <tr
      tabIndex={0}
      data-file-row
      data-directory-path={directory.path}
      aria-label={`Folder ${directory.name}`}
      aria-selected={selected}
      onClick={onSelect}
      onDoubleClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === "Enter" || ((e.metaKey || e.ctrlKey) && e.key === "ArrowDown")) {
          e.preventDefault();
          e.stopPropagation();
          onOpen();
        } else if (e.key === " ") {
          e.preventDefault();
          onSelect();
        }
      }}
      className={cn("cursor-pointer outline-none hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring", selected && "bg-accent/60", directory.state === "missing" && "opacity-60")}
    >
      <td className="border-b border-border/60 px-4 py-2.5" />
      <td className="border-b border-border/60 px-4 py-2.5">
        <button
          type="button"
          aria-label={`Open folder ${directory.name}`}
          title={directory.path}
          className="flex min-h-9 w-full min-w-0 items-center gap-3 rounded text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Folder size={20} aria-hidden className="shrink-0 text-amber-600 dark:text-amber-400" />
          <span className="truncate font-medium">{directory.name}</span>
          {directory.state === "missing" && <Badge variant="destructive">missing</Badge>}
        </button>
      </td>
      <td className="border-b border-border/60 px-4 py-2.5 text-right text-muted-foreground">—</td>
      <td title={directory.modified_at ? new Date(directory.modified_at).toLocaleString() : undefined} className="files-modified-column truncate border-b border-border/60 px-4 py-2.5 text-muted-foreground">{fileDate(directory.modified_at)}</td>
      <td title={directory.file_created_at ? new Date(directory.file_created_at).toLocaleString() : "File creation time unavailable"} className="files-created-column truncate border-b border-border/60 px-4 py-2.5 tabular-nums text-muted-foreground">{fileDate(directory.file_created_at)}</td>
      <td className="files-tags-column border-b border-border/60 px-4 py-2.5 text-xs text-muted-foreground">Folder</td>
    </tr>
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
        "flex min-h-9 w-full items-center gap-2 rounded-md px-3 text-left text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        active
          ? "bg-accent text-accent-foreground"
          : "text-muted-foreground hover:bg-muted hover:text-foreground",
      )}
    >
      {icon}
      {children}
      <span
        className={cn(
          "ml-auto text-xs tabular-nums",
          active ? "text-accent-foreground" : "text-muted-foreground",
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
  showPath,
  onToggleCheck,
  onSelect,
  onPreview,
  onOpenDirectory,
}: {
  resource: Resource;
  selected: boolean;
  checked: boolean;
  showPath: boolean;
  onToggleCheck: (id: number) => void;
  onSelect: (r: Resource, event?: React.MouseEvent) => void;
  onPreview: (r: Resource) => void;
  onOpenDirectory: (r: Resource) => void;
}) {
  const meta = kindMeta(resource.kind);
  const Icon = meta.icon;
  const dir = parentDir(resource);
  return (
    <tr
      data-file-row
      data-resource-id={resource.id}
      aria-selected={selected || checked}
      onClick={(event) => onSelect(resource, event)}
      onDoubleClick={() => {
        if (resource.is_dir) onOpenDirectory(resource);
        else if (resource.state === "present") onPreview(resource);
      }}
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.target !== e.currentTarget) return;
        if (e.key === "Enter" || e.key === " " || ((e.metaKey || e.ctrlKey) && e.key === "ArrowDown")) {
          e.preventDefault();
          e.stopPropagation();
          if (resource.is_dir && e.key !== " ") onOpenDirectory(resource);
          else if (!resource.is_dir && resource.state === "present") onPreview(resource);
          else onSelect(resource);
        }
      }}
      className={cn(
        "group cursor-pointer outline-none transition-colors",
        selected || checked ? "bg-accent/60" : "hover:bg-muted/50",
        "focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring",
        resource.state === "missing" && "opacity-60",
      )}
    >
      <td
        className="border-b border-border/60 px-3 py-2.5"
        onClick={(e) => e.stopPropagation()}
        onDoubleClick={(e) => e.stopPropagation()}
      >
        <label className="inline-flex h-6 w-6 cursor-pointer items-center justify-center">
          <input
            type="checkbox"
            aria-label={`Select ${resource.name}`}
            className="h-4 w-4 cursor-pointer accent-primary"
            checked={checked}
            onChange={() => onToggleCheck(resource.id)}
          />
        </label>
      </td>
      <td
        className={cn(
          "border-b border-border/60 px-4 py-2.5",
          selected && "border-l-2 border-l-primary pl-[14px]",
        )}
      >
        <div className="flex items-center gap-3">
          <Icon size={20} aria-hidden className={cn("shrink-0", meta.chip.split(" ").filter((token) => !token.startsWith("bg-")).join(" "))} />
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span title={resource.name} className="truncate font-medium text-foreground">
                {resource.name}
              </span>
              {resource.state === "missing" && (
                <Badge variant="destructive">missing</Badge>
              )}
            </div>
            {showPath && dir && (
              <span title={dir} className="block truncate text-xs text-muted-foreground">
                {dir}
              </span>
            )}
          </div>
        </div>
      </td>
      <td className="whitespace-nowrap border-b border-border/60 px-4 py-2.5 text-right tabular-nums text-muted-foreground">
        {resource.is_dir ? "—" : bytes(resource.size)}
      </td>
      <td title={resource.modified_at ? new Date(resource.modified_at).toLocaleString() : undefined} className="files-modified-column truncate border-b border-border/60 px-4 py-2.5 tabular-nums text-muted-foreground">
        {fileDate(resource.modified_at)}
      </td>
      <td title={resource.file_created_at ? new Date(resource.file_created_at).toLocaleString() : "File creation time unavailable"} className="files-created-column truncate border-b border-border/60 px-4 py-2.5 tabular-nums text-muted-foreground">
        {fileDate(resource.file_created_at)}
      </td>
      <td className="files-tags-column border-b border-border/60 px-4 py-2.5">
        <div className="flex flex-wrap gap-1">
          {resource.tags.map((t) => (
            <Badge key={t} variant="primary" className="max-w-full" title={t}>
              <span className="truncate">{t}</span>
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
  onClearMissing,
  onRename,
  onMove,
  onDelete,
  onClose,
  onPreview,
}: {
  resource: Resource;
  onAddTag: (r: Resource, tag: string) => void;
  onRemoveTag: (r: Resource, tag: string) => void;
  onClearMissing: (r: Resource) => void;
  onRename: (r: Resource, name: string) => void;
  onMove: (r: Resource, dest: string) => void;
  onDelete: (r: Resource) => void;
  onClose: () => void;
  onPreview: () => void;
}) {
  const [tag, setTag] = useState("");
  const [name, setName] = useState(resource.name);
  const [dest, setDest] = useState("");
  const meta = kindMeta(resource.kind);
  const Icon = meta.icon;

  return (
    <section aria-label="File details" className="space-y-4 p-4">
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
        <Button size="icon-sm" variant="ghost" aria-label="Close file details" onClick={onClose}>
          <X size={16} />
        </Button>
      </div>
      <div className="break-all rounded-md bg-muted/50 p-2 font-mono text-xs text-muted-foreground">
        {resource.path}
      </div>
      <dl className="space-y-1 text-xs">
        <dt className="font-medium text-muted-foreground">Created on Host</dt>
        <dd>{resource.file_created_at ? <time dateTime={resource.file_created_at}>{new Date(resource.file_created_at).toLocaleString()}</time> : "Unavailable — scan the Managed Root to refresh metadata."}</dd>
      </dl>

      <div className="space-y-1.5">
        <label htmlFor="resource-tag" className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
          <Tag size={13} aria-hidden /> Tags
        </label>
        <div className="flex flex-wrap gap-1">
          {resource.tags.map((t) => (
            <button
              type="button"
              key={t}
              aria-label={`Remove tag ${t}`}
              className="inline-flex min-h-6 items-center gap-1 rounded-full bg-primary/10 px-2 text-xs font-medium text-primary transition hover:bg-destructive/15 hover:text-destructive focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              onClick={() => onRemoveTag(resource, t)}
            >
              {t}
              <X size={11} aria-hidden />
            </button>
          ))}
          {resource.tags.length === 0 && (
            <span className="text-xs text-muted-foreground">No tags yet.</span>
          )}
        </div>
        <div className="flex gap-2">
          <Input
            id="resource-tag"
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
          {!resource.is_dir && (
            <Button className="w-full bg-accent-foreground hover:bg-accent-foreground/90" onClick={onPreview}>
              <Eye size={15} aria-hidden /> Preview / edit
            </Button>
          )}
          {!resource.is_dir && <a
            href={`/api/files/resources/${resource.id}/download`}
            className="inline-flex h-8 w-full items-center justify-center gap-2 rounded-md bg-muted px-3 text-sm font-medium text-foreground transition-colors hover:bg-muted/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background"
          >
            <Download size={15} /> Download
          </a>}


          <details className="space-y-2">
            <summary className="cursor-pointer rounded py-1 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">Rename or move</summary>
            <label htmlFor="resource-name" className="text-xs font-medium text-muted-foreground">Name</label>
            <div className="flex gap-2">
              <Input id="resource-name" className="h-8" value={name} onChange={(e) => setName(e.target.value)} />
              <Button size="sm" onClick={() => onRename(resource, name)}>
                Rename
              </Button>
            </div>
            <label htmlFor="resource-destination" className="block text-xs font-medium text-muted-foreground">
              Move to (absolute path)
            </label>
            <div className="flex gap-2">
              <Input
                id="resource-destination"
                className="h-8 font-mono"
                placeholder="/managed/root/dest"
                value={dest}
                onChange={(e) => setDest(e.target.value)}
              />
              <Button size="sm" onClick={() => onMove(resource, dest)}>
                Move
              </Button>
            </div>
          </details>

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
    </section>
  );
}
