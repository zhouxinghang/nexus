import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { Button, Card, Input, Badge, Textarea } from "@/components/ui";
import { cn } from "@/lib/api";

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
    await api.post(`/api/files/roots/${id}/scan`);
    loadResources();
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
      <div>
        <h1 className="text-2xl font-semibold">Files</h1>
        <p className="text-sm text-muted-foreground">
          Manage Resources across your Managed Roots.
        </p>
      </div>

      <Card className="space-y-3 p-4">
        <h2 className="font-semibold">Managed Roots</h2>
        <form onSubmit={addRoot} className="flex gap-2">
          <Input
            placeholder="/absolute/path/to/directory"
            value={newRoot}
            onChange={(e) => setNewRoot(e.target.value)}
          />
          <Button type="submit">Add root</Button>
        </form>
        {error && <p className="text-sm text-destructive">{error}</p>}
        <div className="space-y-2">
          {roots.map((r) => (
            <div key={r.id} className="flex items-center justify-between text-sm">
              <span className="font-mono">{r.path}</span>
              <div className="flex gap-2">
                <Button size="sm" variant="outline" onClick={() => scan(r.id)}>
                  Scan
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  className="text-destructive"
                  onClick={() => removeRoot(r.id)}
                >
                  Remove
                </Button>
              </div>
            </div>
          ))}
          {roots.length === 0 && (
            <p className="text-sm text-muted-foreground">No roots configured.</p>
          )}
        </div>
      </Card>

      <div className="flex flex-wrap gap-2">
        <select
          className="h-9 rounded-md border border-border bg-background px-2 text-sm"
          value={rootFilter}
          onChange={(e) => setRootFilter(e.target.value === "" ? "" : Number(e.target.value))}
        >
          <option value="">All roots</option>
          {roots.map((r) => (
            <option key={r.id} value={r.id}>
              {r.path}
            </option>
          ))}
        </select>
        <Input
          className="max-w-xs"
          placeholder="Filter by tag…"
          value={tagFilter}
          onChange={(e) => setTagFilter(e.target.value)}
        />
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        <Card className="p-0 lg:col-span-2">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-muted-foreground">
                <th className="p-2">Name</th>
                <th>Size</th>
                <th>Modified</th>
                <th>Tags</th>
              </tr>
            </thead>
            <tbody>
              {resources.map((r) => (
                <tr
                  key={r.id}
                  onClick={() => setSelected(r)}
                  className={cn(
                    "cursor-pointer border-t border-border hover:bg-muted",
                    r.state === "missing" && "opacity-40",
                  )}
                >
                  <td className="p-2">
                    {r.name}
                    {r.state === "missing" && <Badge className="ml-2">missing</Badge>}
                  </td>
                  <td>{bytes(r.size)}</td>
                  <td>{r.modified_at ? new Date(r.modified_at).toLocaleDateString() : "—"}</td>
                  <td>
                    {r.tags.map((t) => (
                      <Badge key={t} className="mr-1">
                        {t}
                      </Badge>
                    ))}
                  </td>
                </tr>
              ))}
              {resources.length === 0 && (
                <tr>
                  <td colSpan={4} className="p-4 text-sm text-muted-foreground">
                    No resources. Add a root and scan it.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
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
    <Card className="space-y-4 p-4">
      <div className="flex items-start justify-between">
        <h2 className="font-semibold">{resource.name}</h2>
        <Button size="sm" variant="ghost" onClick={onClose}>
          ✕
        </Button>
      </div>
      <div className="break-all font-mono text-xs text-muted-foreground">{resource.path}</div>

      {resource.state === "missing" ? (
        <div className="space-y-2">
          <p className="text-sm text-destructive">This file is missing from disk.</p>
          <Button size="sm" variant="outline" onClick={() => onClearMissing(resource)}>
            Clear from index
          </Button>
        </div>
      ) : (
        <>
          <div>
            <a
              className="text-sm underline"
              href={`/api/files/resources/${resource.id}/download`}
            >
              Download / open
            </a>
          </div>

          <div className="space-y-1">
            <label className="text-xs font-medium">Tags</label>
            <div className="flex flex-wrap gap-1">
              {resource.tags.map((t) => (
                <Badge key={t} className="cursor-pointer" onClick={() => onRemoveTag(resource, t)}>
                  {t} ✕
                </Badge>
              ))}
            </div>
            <div className="flex gap-2">
              <Input
                className="h-8"
                placeholder="add tag"
                value={tag}
                onChange={(e) => setTag(e.target.value)}
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

          <div className="space-y-1">
            <label className="text-xs font-medium">Note</label>
            <Textarea rows={3} value={note} onChange={(e) => setNote(e.target.value)} />
            <Button size="sm" variant="outline" onClick={() => onSaveNote(resource, note)}>
              Save note
            </Button>
          </div>

          <div className="space-y-1">
            <label className="text-xs font-medium">Rename</label>
            <div className="flex gap-2">
              <Input className="h-8" value={name} onChange={(e) => setName(e.target.value)} />
              <Button size="sm" onClick={() => onRename(resource, name)}>
                Rename
              </Button>
            </div>
          </div>

          <div className="space-y-1">
            <label className="text-xs font-medium">Move to (absolute path)</label>
            <div className="flex gap-2">
              <Input
                className="h-8"
                placeholder="/managed/root/dest"
                value={dest}
                onChange={(e) => setDest(e.target.value)}
              />
              <Button size="sm" onClick={() => onMove(resource, dest)}>
                Move
              </Button>
            </div>
          </div>

          <Button
            size="sm"
            variant="destructive"
            onClick={() => {
              if (confirm("Permanently delete this file?")) onDelete(resource);
            }}
          >
            Delete file (permanent)
          </Button>
        </>
      )}
    </Card>
  );
}
