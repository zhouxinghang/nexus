import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import {
  Button,
  Card,
  Input,
  Modal,
  PageHeader,
  EmptyState,
  Skeleton,
  Badge,
} from "@/components/ui";
import { cn } from "@/lib/api";
import {
  Compass,
  Plus,
  Trash2,
  Pencil,
  ExternalLink,
  RefreshCw,
  Type,
  Link2,
  Smile,
} from "lucide-react";

type Health = "unknown" | "up" | "down";

type Bookmark = {
  id: number;
  name: string;
  url: string;
  icon: string;
  health: Health;
  last_status: number | null;
  last_latency_ms: number | null;
  last_checked_at: string | null;
};

const health: Record<
  Health,
  { label: string; variant: "success" | "destructive" | "default"; dot: string }
> = {
  up: { label: "Up", variant: "success", dot: "bg-success" },
  down: { label: "Down", variant: "destructive", dot: "bg-destructive" },
  unknown: { label: "Unchecked", variant: "default", dot: "bg-muted-foreground/40" },
};

// hostOf extracts a bookmark's hostname (no port) for the favicon lookup.
function hostOf(url: string): string {
  try {
    return new URL(url).hostname;
  } catch {
    return url;
  }
}

// hostLabel is what we show under the name. URL.host keeps a non-standard port
// (e.g. example.com:8443) but omits the default :80/:443, so a non-standard
// port surfaces in the UI without cluttering ordinary links.
function hostLabel(url: string): string {
  try {
    return new URL(url).host;
  } catch {
    return url;
  }
}

function faviconFor(url: string): string {
  return `https://www.google.com/s2/favicons?domain=${encodeURIComponent(hostOf(url))}&sz=64`;
}

function checkedLabel(iso: string | null): string {
  if (!iso) return "never checked";
  const diff = Date.now() - new Date(iso).getTime();
  const mins = Math.round(diff / 60000);
  if (mins < 1) return "checked just now";
  if (mins < 60) return `checked ${mins}m ago`;
  const hrs = Math.round(mins / 60);
  if (hrs < 24) return `checked ${hrs}h ago`;
  return `checked ${Math.round(hrs / 24)}d ago`;
}

// BookmarkIcon renders the Owner's chosen icon: a short string (emoji) inline,
// an image URL as an <img>, or a favicon fetched from the link's host.
function BookmarkIcon({ bookmark }: { bookmark: Bookmark }) {
  const [failed, setFailed] = useState(false);
  const icon = bookmark.icon.trim();
  const isImageUrl = /^https?:\/\//i.test(icon);
  const isEmojiish = icon !== "" && !isImageUrl && icon.length <= 4;

  if (isEmojiish) {
    return <span className="text-xl leading-none">{icon}</span>;
  }
  const src = !failed ? (isImageUrl ? icon : faviconFor(bookmark.url)) : "";
  if (src) {
    return (
      <img
        src={src}
        alt=""
        className="h-6 w-6 rounded"
        onError={() => setFailed(true)}
      />
    );
  }
  // Final fallback: first letter of the name.
  return (
    <span className="text-sm font-semibold text-muted-foreground">
      {bookmark.name.charAt(0).toUpperCase()}
    </span>
  );
}

export default function Bookmarks() {
  const [bookmarks, setBookmarks] = useState<Bookmark[]>([]);
  const [loading, setLoading] = useState(true);
  const [name, setName] = useState("");
  const [url, setUrl] = useState("");
  const [icon, setIcon] = useState("");
  const [busy, setBusy] = useState(false);
  const [checkingAll, setCheckingAll] = useState(false);
  const [checkingId, setCheckingId] = useState<number | null>(null);
  const [editing, setEditing] = useState<Bookmark | null>(null);
  const pollTimer = useRef<ReturnType<typeof setInterval> | null>(null);

  async function load() {
    setBookmarks(await api.get<Bookmark[]>("/api/bookmarks"));
  }

  useEffect(() => {
    load().finally(() => setLoading(false));
    // Pick up health transitions produced by the background checker.
    pollTimer.current = setInterval(() => {
      load().catch(() => {});
    }, 60000);
    return () => {
      if (pollTimer.current) clearInterval(pollTimer.current);
    };
  }, []);

  async function create(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim() || !url.trim()) return;
    setBusy(true);
    try {
      await api.post("/api/bookmarks", { name, url, icon });
      setName("");
      setUrl("");
      setIcon("");
      await load();
    } finally {
      setBusy(false);
    }
  }

  async function remove(id: number) {
    await api.del(`/api/bookmarks/${id}`);
    setEditing((e) => (e && e.id === id ? null : e));
    load();
  }

  async function checkOne(id: number) {
    setCheckingId(id);
    try {
      const updated = await api.post<Bookmark>(`/api/bookmarks/${id}/check`, null);
      setBookmarks((prev) => prev.map((b) => (b.id === id ? updated : b)));
    } finally {
      setCheckingId(null);
    }
  }

  async function checkAll() {
    setCheckingAll(true);
    try {
      setBookmarks(await api.post<Bookmark[]>("/api/bookmarks/check", null));
    } finally {
      setCheckingAll(false);
    }
  }

  return (
    <div className="space-y-6">
      <PageHeader
        icon={Compass}
        title="Bookmarks"
        description="A navigation board of your sites. Click a card to open it; Nexus checks each link's health periodically."
        actions={
          bookmarks.length > 0 ? (
            <Button variant="outline" onClick={checkAll} loading={checkingAll}>
              <RefreshCw size={15} /> Check all
            </Button>
          ) : undefined
        }
      />

      <Card className="p-4">
        <form onSubmit={create} className="flex flex-wrap gap-2">
          <Input
            className="min-w-[10rem] flex-1"
            placeholder="Name"
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <Input
            className="min-w-[14rem] flex-[2]"
            placeholder="https://example.com"
            value={url}
            onChange={(e) => setUrl(e.target.value)}
          />
          <Input
            className="w-28"
            placeholder="Icon"
            title="Emoji or image URL (optional)"
            value={icon}
            onChange={(e) => setIcon(e.target.value)}
          />
          <Button type="submit" loading={busy} disabled={!name.trim() || !url.trim()}>
            <Plus size={15} /> Add
          </Button>
        </form>
      </Card>

      {loading ? (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {Array.from({ length: 6 }).map((_, i) => (
            <Skeleton key={i} className="h-[5.5rem]" />
          ))}
        </div>
      ) : bookmarks.length === 0 ? (
        <EmptyState
          icon={Compass}
          title="No bookmarks yet"
          description="Add your first site above to start your navigation board."
        />
      ) : (
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {bookmarks.map((b) => {
            const hm = health[b.health];
            return (
              <Card
                key={b.id}
                interactive
                className="group relative flex items-center gap-3 p-4"
              >
                <a
                  href={b.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex min-w-0 flex-1 items-center gap-3 outline-none"
                >
                  <span className="flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-border bg-muted">
                    <BookmarkIcon bookmark={b} />
                  </span>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-1.5">
                      <span className="truncate font-medium">{b.name}</span>
                      <ExternalLink
                        size={13}
                        className="shrink-0 text-muted-foreground opacity-0 transition group-hover:opacity-100"
                      />
                    </div>
                    <p className="truncate text-xs text-muted-foreground">
                      {hostLabel(b.url)}
                    </p>
                    <div className="mt-1 flex items-center gap-1.5">
                      <span
                        className={cn("h-1.5 w-1.5 rounded-full", hm.dot)}
                        aria-hidden
                      />
                      <span className="text-[11px] text-muted-foreground">
                        {hm.label}
                        {b.health !== "unknown" && (
                          <> · {checkedLabel(b.last_checked_at)}</>
                        )}
                      </span>
                    </div>
                  </div>
                </a>

                <div className="absolute right-2 top-2 flex items-center gap-0.5 opacity-0 transition group-hover:opacity-100 focus-within:opacity-100">
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Recheck health"
                    title="Recheck health"
                    loading={checkingId === b.id}
                    onClick={() => checkOne(b.id)}
                  >
                    {checkingId !== b.id && <RefreshCw size={14} />}
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon-sm"
                    aria-label="Edit bookmark"
                    title="Edit bookmark"
                    onClick={() => setEditing(b)}
                  >
                    <Pencil size={14} />
                  </Button>
                </div>
              </Card>
            );
          })}
        </div>
      )}

      {editing && (
        <BookmarkEditor
          key={editing.id}
          bookmark={editing}
          onClose={() => setEditing(null)}
          onSaved={(updated) =>
            setBookmarks((prev) =>
              prev.map((b) => (b.id === updated.id ? updated : b)),
            )
          }
          onDelete={remove}
        />
      )}
    </div>
  );
}

function BookmarkEditor({
  bookmark,
  onClose,
  onSaved,
  onDelete,
}: {
  bookmark: Bookmark;
  onClose: () => void;
  onSaved: (b: Bookmark) => void;
  onDelete: (id: number) => void;
}) {
  const [name, setName] = useState(bookmark.name);
  const [url, setUrl] = useState(bookmark.url);
  const [icon, setIcon] = useState(bookmark.icon);
  const [saving, setSaving] = useState(false);

  const dirty =
    name !== bookmark.name || url !== bookmark.url || icon !== bookmark.icon;

  async function save() {
    if (!name.trim() || !url.trim() || saving) return;
    setSaving(true);
    try {
      const updated = await api.patch<Bookmark>(`/api/bookmarks/${bookmark.id}`, {
        name: name.trim(),
        url: url.trim(),
        icon: icon.trim(),
      });
      onSaved(updated);
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
      title="Edit bookmark"
      description="Update the name, link, or icon."
      footer={
        <>
          <Button
            variant="destructive"
            size="sm"
            className="mr-auto"
            onClick={() => {
              if (confirm("Delete this bookmark?")) onDelete(bookmark.id);
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
            disabled={!name.trim() || !url.trim() || !dirty}
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
            <Type size={13} /> Name
          </label>
          <Input value={name} onChange={(e) => setName(e.target.value)} autoFocus />
        </div>
        <div className="space-y-1.5">
          <label className="flex h-5 items-center gap-1.5 text-xs font-medium text-muted-foreground">
            <Link2 size={13} /> Link
          </label>
          <Input
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="https://example.com"
          />
        </div>
        <div className="space-y-1.5">
          <label className="flex h-5 items-center gap-1.5 text-xs font-medium text-muted-foreground">
            <Smile size={13} /> Icon
          </label>
          <Input
            value={icon}
            onChange={(e) => setIcon(e.target.value)}
            placeholder="Emoji or image URL — leave blank for the site favicon"
          />
        </div>
        <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <Badge variant={health[bookmark.health].variant}>
            {health[bookmark.health].label}
          </Badge>
          {bookmark.last_status != null && <>HTTP {bookmark.last_status}</>}
          {bookmark.last_latency_ms != null && (
            <> · {bookmark.last_latency_ms} ms</>
          )}
        </p>
      </div>
    </Modal>
  );
}
