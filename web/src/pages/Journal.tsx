import { useEffect, useId, useRef, useState } from "react";
import { api } from "@/lib/api";
import { copyText } from "@/lib/clipboard";
import { useToast } from "@/lib/toast";
import { Button, Card, Textarea, Input, EmptyState, Skeleton } from "@/components/ui";
import { BookText, Check, Copy, Search, X } from "lucide-react";

type Entry = { id: number; body: string; created_at: string };
type EntryPage = { items: Entry[]; next_cursor: string | null };

export default function Journal() {
  const [entries, setEntries] = useState<Entry[]>([]);
  const [body, setBody] = useState("");
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [copiedId, setCopiedId] = useState<number | null>(null);
  const [manualCopy, setManualCopy] = useState<Entry | null>(null);
  const toast = useToast();
  const activeQuery = useRef("");
  const copyTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const copying = useRef(false);
  const copyVersion = useRef(0);

  function pagePath(q: string, cursor?: string) {
    const params = new URLSearchParams();
    if (q) params.set("q", q);
    if (cursor) params.set("cursor", cursor);
    const qs = params.toString();
    return `/api/journal/entries${qs ? `?${qs}` : ""}`;
  }

  // load fetches the first page (newest entries) for the current search.
  async function load(q = "") {
    activeQuery.current = q;
    setLoading(true);
    try {
      const page = await api.get<EntryPage>(pagePath(q));
      setEntries(page.items);
      setNextCursor(page.next_cursor);
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setLoading(false);
    }
  }

  // loadMore appends the next (older) page after the last loaded entry.
  async function loadMore() {
    if (!nextCursor || loadingMore) return;
    const q = activeQuery.current;
    setLoadingMore(true);
    try {
      const page = await api.get<EntryPage>(pagePath(q, nextCursor));
      // Ignore a page that resolves after the search changed underneath it.
      if (activeQuery.current !== q) return;
      setEntries((prev) => [...prev, ...page.items]);
      setNextCursor(page.next_cursor);
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setLoadingMore(false);
    }
  }

  useEffect(() => {
    load();
    return () => {
      if (copyTimer.current) clearTimeout(copyTimer.current);
      copyVersion.current++;
    };
  }, []);

  async function copyEntry(entry: Entry) {
    if (copying.current) return;
    copying.current = true;
    const version = ++copyVersion.current;
    setCopiedId(null);
    if (copyTimer.current) clearTimeout(copyTimer.current);
    try {
      const copied = await copyText(entry.body);
      if (version !== copyVersion.current) return;
      if (copied) {
        setCopiedId(entry.id);
        copyTimer.current = setTimeout(() => setCopiedId(null), 1500);
        toast.success("Copied to clipboard");
      } else {
        setManualCopy(entry);
      }
    } catch {
      if (version === copyVersion.current) setManualCopy(entry);
    } finally {
      copying.current = false;
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
      toast.success("Entry appended");
    } catch (err) {
      toast.error((err as Error).message);
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
                  className="opacity-100 transition-opacity md:opacity-0 md:group-hover:opacity-100 focus-visible:opacity-100"
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
        {!loading && entries.length > 0 && (
          <div className="flex justify-center pt-1">
            {nextCursor ? (
              <Button variant="outline" size="sm" loading={loadingMore} onClick={loadMore}>
                Load older entries
              </Button>
            ) : (
              <span className="text-xs text-muted-foreground">
                {activeQuery.current ? "End of matching entries" : "You've reached the first entry"}
              </span>
            )}
          </div>
        )}
      </div>
      {manualCopy && <ManualCopyDialog text={manualCopy.body} onClose={() => setManualCopy(null)} />}
    </div>
  );
}

function ManualCopyDialog({ text, onClose }: { text: string; onClose: () => void }) {
  const dialogRef = useRef<HTMLDialogElement>(null);
  const fieldRef = useRef<HTMLTextAreaElement>(null);
  const id = useId();

  function selectText() {
    fieldRef.current?.focus({ preventScroll: true });
    fieldRef.current?.select();
  }

  useEffect(() => {
    const dialog = dialogRef.current!;
    const trigger = document.activeElement as HTMLElement | null;
    const previousOverflow = document.body.style.overflow;
    dialog.showModal();
    document.body.style.overflow = "hidden";
    selectText();
    return () => {
      dialog.close();
      document.body.style.overflow = previousOverflow;
      if (trigger?.isConnected) trigger.focus({ preventScroll: true });
    };
  }, []);

  return (
    <dialog
      ref={dialogRef}
      aria-labelledby={`${id}-title`}
      aria-describedby={`${id}-help`}
      className="max-h-[85dvh] w-[calc(100%-2rem)] max-w-xl overflow-auto rounded-xl border border-border bg-card p-0 text-card-foreground shadow-pop backdrop:bg-foreground/40"
      onCancel={(event) => { event.preventDefault(); onClose(); }}
    >
      <div className="flex items-center justify-between gap-3 border-b border-border px-5 py-3">
        <h2 id={`${id}-title`} className="font-semibold">Copy Journal Entry manually</h2>
        <Button size="icon-sm" variant="ghost" aria-label="Close manual copy" onClick={onClose}>
          <X size={16} aria-hidden />
        </Button>
      </div>
      <div className="space-y-3 p-5">
        <p id={`${id}-help`} className="text-sm text-muted-foreground">
          Your browser blocked automatic copying. Press Ctrl/Cmd+C, or use your device's Copy action on the selected text.
        </p>
        <label htmlFor={`${id}-text`} className="block text-sm font-medium">Journal Entry text</label>
        <textarea
          ref={fieldRef}
          id={`${id}-text`}
          aria-describedby={`${id}-help`}
          value={text}
          readOnly
          spellCheck={false}
          className="h-40 max-h-[35dvh] w-full resize-none rounded-md border border-input bg-background px-3 py-2 text-base leading-relaxed outline-none focus-visible:border-ring focus-visible:ring-2 focus-visible:ring-ring/30 sm:text-sm"
        />
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={selectText}>Select all text</Button>
          <Button onClick={onClose}>Done</Button>
        </div>
      </div>
    </dialog>
  );
}
