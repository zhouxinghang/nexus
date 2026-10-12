import { useEffect, useState } from "react";
import { api } from "@/lib/api";
import { Button, Card, Textarea, Input } from "@/components/ui";

type Entry = { id: number; body: string; created_at: string };

export default function Journal() {
  const [entries, setEntries] = useState<Entry[]>([]);
  const [body, setBody] = useState("");
  const [query, setQuery] = useState("");

  async function load(q = "") {
    const path = q ? `/api/journal/entries?q=${encodeURIComponent(q)}` : "/api/journal/entries";
    setEntries(await api.get<Entry[]>(path));
  }

  useEffect(() => {
    load();
  }, []);

  async function append(e: React.FormEvent) {
    e.preventDefault();
    if (!body.trim()) return;
    await api.post("/api/journal/entries", { body });
    setBody("");
    load(query);
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h1 className="text-2xl font-semibold">Journal</h1>
        <p className="text-sm text-muted-foreground">
          An append-only, immutable record. Entries can't be edited or deleted.
        </p>
      </div>

      <Card className="p-4">
        <form onSubmit={append} className="space-y-3">
          <Textarea
            rows={3}
            placeholder="What happened?"
            value={body}
            onChange={(e) => setBody(e.target.value)}
          />
          <div className="flex justify-end">
            <Button type="submit">Append entry</Button>
          </div>
        </form>
      </Card>

      <div className="flex gap-2">
        <Input
          placeholder="Search entries…"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") load(query);
          }}
        />
        <Button variant="outline" onClick={() => load(query)}>
          Search
        </Button>
        {query && (
          <Button
            variant="ghost"
            onClick={() => {
              setQuery("");
              load("");
            }}
          >
            Clear
          </Button>
        )}
      </div>

      <div className="space-y-3">
        {entries.map((e) => (
          <Card key={e.id} className="p-4">
            <div className="mb-1 text-xs text-muted-foreground">
              {new Date(e.created_at).toLocaleString()}
            </div>
            <div className="whitespace-pre-wrap text-sm">{e.body}</div>
          </Card>
        ))}
        {entries.length === 0 && (
          <p className="text-sm text-muted-foreground">No entries yet.</p>
        )}
      </div>
    </div>
  );
}
