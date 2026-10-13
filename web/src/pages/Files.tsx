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
  FolderPlus,
  RefreshCw,
  Trash2,
  Download,
  X,
  Tag,
  Folder,
  AlertTriangle,
  Search,
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

export default function Files() {
  const [roots, setRoots] = useState<Root[]>([]);
  const [resources, setResources] = useState<Resource[]>([]);
  const [newRoot, setNewRoot] = useState("");
  const [rootFilter, setRootFilter] = useState<number | "">("");
  const [tagFilter, setTagFilter] = useState("");
  const [selected, setSelected] = useState<Resource | null>(null);
  const [error, setError] = useState("");
  const [scanning, setScanning] = useState<number | null>(null);

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
        </div>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card className="overflow-hidden p-0 lg:col-span-2">
          {resources.length === 0 ? (
            <EmptyState
              icon={FileText}
              title="No resources"
              description="Add a Managed Root above and scan it to index its files."
            />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-border bg-muted/40 text-left text-xs text-muted-foreground">
                    <th className="p-3 font-medium">Name</th>
                    <th className="p-3 font-medium">Size</th>
                    <th className="p-3 font-medium">Modified</th>
                    <th className="p-3 font-medium">Tags</th>
                  </tr>
                </thead>
                <tbody>
                  {resources.map((r) => (
                    <tr
                      key={r.id}
                      onClick={() => setSelected(r)}
                      className={cn(
                        "cursor-pointer border-b border-border/60 transition-colors last:border-0 hover:bg-muted/50",
                        selected?.id === r.id && "bg-accent/50",
                        r.state === "missing" && "opacity-50",
                      )}
                    >
                      <td className="p-3">
                        <div className="flex items-center gap-2">
                          <FileText
                            size={15}
                            className="shrink-0 text-muted-foreground"
                          />
                          <span className="truncate font-medium">{r.name}</span>
                          {r.state === "missing" && (
                            <Badge variant="destructive">missing</Badge>
                          )}
                        </div>
                      </td>
                      <td className="p-3 tabular-nums text-muted-foreground">
                        {bytes(r.size)}
                      </td>
                      <td className="p-3 text-muted-foreground">
                        {r.modified_at
                          ? new Date(r.modified_at).toLocaleDateString()
                          : "—"}
                      </td>
                      <td className="p-3">
                        <div className="flex flex-wrap gap-1">
                          {r.tags.map((t) => (
                            <Badge key={t} variant="primary">
                              {t}
                            </Badge>
                          ))}
                        </div>
                      </td>
                    </tr>
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

  return (
    <Card className="h-fit animate-scale-in space-y-4 p-4 lg:sticky lg:top-8">
      <div className="flex items-start justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <FileText size={16} className="shrink-0 text-muted-foreground" />
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
