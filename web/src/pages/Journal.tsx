import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import { Button, Card, Textarea, Input, EmptyState, Skeleton } from "@/components/ui";
import { BookText, Check, Copy, Search, X } from "lucide-react";

type Entry = { id: number; body: string; created_at: string };

export default function Journal() {
  const [entries, setEntries] = useState<Entry[]>([]);
  const [body, setBody] = useState("");
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [copiedId, setCopiedId] = useState<number | null>(null);
  const activeQuery = useRef("");
  const copyTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  async function load(q = "") {
    activeQuery.current = q;
    setLoading(true);
    const path = q
      ? `/api/journal/entries?q=${encodeURIComponent(q)}`
      : "/api/journal/entries";
    try {
      setEntries(await api.get<Entry[]>(path));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    load();
    return () => {
      if (copyTimer.current) clearTimeout(copyTimer.current);
    };
  }, []);

  async function copyEntry(entry: Entry) {
    try {
      await navigator.clipboard.writeText(entry.body);
      setCopiedId(entry.id);
      if (copyTimer.current) clearTimeout(copyTimer.current);
      copyTimer.current = setTimeout(() => setCopiedId(null), 1500);
    } catch {
      // Clipboard unavailable (e.g. non-secure context); ignore.
    }
  }

  async function append(e: React.FormEvent) {
    e.preventDefault();
    if (!body.trim()) return;
    setBusy(true);
    try {
      await api.post("/api/journal/entries", { body });
      setBody("");
      await load(activeQuery.current);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div className="flex items-start gap-3">
        <span className="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-accent text-accent-foreground">
          <BookText size={20} />
        </span>
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Journal</h1>
          <p className="mt-0.5 text-sm text-muted-foreground">
            An append-only, immutable record. Entries can't be edited or deleted.
          </p>
        </div>
      </div>

      <Card className="p-4">
        <form onSubmit={append} className="space-y-3">
          <Textarea
            rows={3}
            placeholder="What happened?"
            value={body}
            onChange={(e) => setBody(e.target.value)}
            onKeyDown={(e) => {
              if ((e.metaKey || e.ctrlKey) && e.key === "Enter") append(e);
            }}
          />
          <div className="flex items-center justify-between">
            <span className="text-xs text-muted-foreground">
              Press ⌘/Ctrl + Enter to append
            </span>
            <Button type="submit" loading={busy} disabled={!body.trim()}>
              Append entry
            </Button>
          </div>
        </form>
      </Card>

      <div className="relative flex gap-2">
        <div className="relative flex-1">
          <Search
            size={15}
            className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            className="pl-9"
            placeholder="Search entries…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") load(query);
            }}
          />
        </div>
        <Button variant="outline" onClick={() => load(query)}>
          Search
        </Button>
        {query && (
          <Button
            variant="ghost"
            size="icon"
            aria-label="Clear search"
            onClick={() => {
              setQuery("");
              load("");
            }}
          >
            <X size={16} />
          </Button>
        )}
      </div>

      <div className="space-y-3">
        {loading ? (
          Array.from({ length: 3 }).map((_, i) => (
            <Card key={i} className="space-y-2 p-4">
              <Skeleton className="h-3 w-28" />
              <Skeleton className="h-4 w-full" />
              <Skeleton className="h-4 w-2/3" />
            </Card>
          ))
        ) : entries.length === 0 ? (
          <EmptyState
            icon={BookText}
            title={activeQuery.current ? "No matching entries" : "No entries yet"}
            description={
              activeQuery.current
                ? "Try a different search term."
                : "Write your first journal entry above."
            }
          />
        ) : (
          entries.map((e) => (
            <Card key={e.id} className="group p-4">
              <div className="mb-1 flex items-center justify-between gap-2">
                <span className="text-xs font-medium text-muted-foreground">
                  {new Date(e.created_at).toLocaleString()}
                </span>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={copiedId === e.id ? "Copied" : "Copy entry"}
                  title={copiedId === e.id ? "Copied" : "Copy entry"}
                  className="opacity-0 transition-opacity group-hover:opacity-100 focus-visible:opacity-100"
                  onClick={() => copyEntry(e)}
                >
                  {copiedId === e.id ? (
                    <Check size={15} className="text-success" />
                  ) : (
                    <Copy size={15} />
                  )}
                </Button>
              </div>
              <div className="whitespace-pre-wrap text-sm leading-relaxed">
                {e.body}
              </div>
            </Card>
          ))
        )}
      </div>
    </div>
  );
}
